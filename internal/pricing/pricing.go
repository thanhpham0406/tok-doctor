package pricing

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

//go:embed embedded_catalog.json
var catalogFS embed.FS

const (
	maxCatalogBytes = 2 << 20
	updateTimeout   = 10 * time.Second

	defaultRemoteCatalogURL = "https://raw.githubusercontent.com/thanhpham0406/tok-doctor/main/pricing/catalog.json"
)

type CostProvenance string

const (
	CostEstimatedAPI CostProvenance = "estimated_api"
	CostReported     CostProvenance = "reported"
	CostUnknown      CostProvenance = "unknown"
)

type CostStatus string

const (
	CostAvailable   CostStatus = "available"
	CostUnavailable CostStatus = "unavailable"
)

type CatalogSource string

const (
	SourceOverride CatalogSource = "override"
	SourceCache    CatalogSource = "cache"
	SourceEmbedded CatalogSource = "embedded"
)

type Catalog struct {
	Version     string           `json:"version"`
	GeneratedAt *time.Time       `json:"generated_at,omitempty"`
	Profiles    []PricingProfile `json:"profiles"`
}

type PricingProfile struct {
	Provider      string        `json:"provider"`
	SKU           string        `json:"sku"`
	Model         string        `json:"model,omitempty"`
	Aliases       []string      `json:"aliases,omitempty"`
	Currency      string        `json:"currency"`
	EffectiveFrom *time.Time    `json:"effective_from,omitempty"`
	Rates         Rates         `json:"rates"`
	Tiers         []PricingTier `json:"tiers,omitempty"`
	Sources       []SourceRef   `json:"sources,omitempty"`
	CatalogSource CatalogSource `json:"-"`
}

type PricingTier struct {
	Name            string `json:"name"`
	UpToInputTokens *int64 `json:"up_to_input_tokens,omitempty"`
	Rates           Rates  `json:"rates"`
}

type Rates struct {
	InputMicrosPerMillion       int64 `json:"input_micros_per_million,omitempty"`
	CachedInputMicrosPerMillion int64 `json:"cached_input_micros_per_million,omitempty"`
	CacheReadMicrosPerMillion   int64 `json:"cache_read_micros_per_million,omitempty"`
	CacheWriteMicrosPerMillion  int64 `json:"cache_write_micros_per_million,omitempty"`
	OutputMicrosPerMillion      int64 `json:"output_micros_per_million,omitempty"`
}

type SourceRef struct {
	URL        string `json:"url"`
	VerifiedAt string `json:"verified_at,omitempty"`
	Note       string `json:"note,omitempty"`
}

type ActiveCatalog struct {
	Catalog Catalog       `json:"catalog"`
	Source  CatalogSource `json:"source"`
	Meta    CatalogMeta   `json:"meta,omitempty"`
}

type CatalogMeta struct {
	Version   string     `json:"version,omitempty"`
	FetchedAt *time.Time `json:"fetched_at,omitempty"`
	ETag      string     `json:"etag,omitempty"`
	URL       string     `json:"url,omitempty"`
}

type Store struct {
	cacheDir     string
	overridePath string
	remoteURL    string
	client       *http.Client
}

type Status struct {
	Source      CatalogSource `json:"source"`
	Version     string        `json:"version,omitempty"`
	GeneratedAt *time.Time    `json:"generated_at,omitempty"`
	FetchedAt   *time.Time    `json:"fetched_at,omitempty"`
	Profiles    int           `json:"profiles"`
	RemoteURL   string        `json:"remote_url"`
	ETag        string        `json:"etag,omitempty"`
}

type UpdateResult struct {
	Updated    bool          `json:"updated"`
	Source     CatalogSource `json:"source"`
	Version    string        `json:"version,omitempty"`
	Profiles   int           `json:"profiles,omitempty"`
	FetchedAt  *time.Time    `json:"fetched_at,omitempty"`
	UsingCache bool          `json:"using_cache,omitempty"`
}

type CostResult struct {
	SessionID   string           `json:"session_id"`
	Model       string           `json:"model,omitempty"`
	Source      string           `json:"source,omitempty"`
	Status      CostStatus       `json:"status"`
	Provenance  CostProvenance   `json:"provenance"`
	Unavailable string           `json:"unavailable,omitempty"`
	Currency    string           `json:"currency,omitempty"`
	Usage       *BillableTokens  `json:"usage,omitempty"`
	Cost        *CostBreakdown   `json:"cost,omitempty"`
	CacheHitPct *BasisPoints     `json:"cache_hit_basis_points,omitempty"`
	Pricing     *ProfileRef      `json:"pricing,omitempty"`
	Catalog     CatalogRef       `json:"catalog"`
	Missing     []MissingProfile `json:"missing,omitempty"`
	Turns       []TurnCost       `json:"turns,omitempty"`
}

type CostCollection struct {
	Status      CostStatus       `json:"status"`
	Provenance  CostProvenance   `json:"provenance"`
	Provider    string           `json:"provider,omitempty"`
	Overlap     string           `json:"overlap,omitempty"`
	Catalog     CatalogRef       `json:"catalog"`
	Groups      []ProviderCost   `json:"groups"`
	Missing     []MissingProfile `json:"missing,omitempty"`
	Unavailable string           `json:"unavailable,omitempty"`
}

type ProviderCost struct {
	Provider   string          `json:"provider"`
	Source     string          `json:"source,omitempty"`
	Sessions   int             `json:"sessions"`
	ModelCalls int             `json:"model_calls"`
	Currency   string          `json:"currency,omitempty"`
	Usage      *BillableTokens `json:"usage,omitempty"`
	Cost       *CostBreakdown  `json:"cost,omitempty"`
	Models     []ModelCost     `json:"models,omitempty"`
}

type ModelCost struct {
	Model      string          `json:"model"`
	SKU        string          `json:"sku"`
	ModelCalls int             `json:"model_calls"`
	Currency   string          `json:"currency,omitempty"`
	Usage      *BillableTokens `json:"usage,omitempty"`
	Cost       *CostBreakdown  `json:"cost,omitempty"`
}

type TurnCost struct {
	ID          string          `json:"id,omitempty"`
	Sequence    int             `json:"sequence"`
	Model       string          `json:"model,omitempty"`
	Status      CostStatus      `json:"status"`
	Provenance  CostProvenance  `json:"provenance"`
	Unavailable string          `json:"unavailable,omitempty"`
	Currency    string          `json:"currency,omitempty"`
	Usage       *BillableTokens `json:"usage,omitempty"`
	Cost        *CostBreakdown  `json:"cost,omitempty"`
	Pricing     *ProfileRef     `json:"pricing,omitempty"`
}

type BillableTokens struct {
	FreshInput int64 `json:"fresh_input"`
	CacheRead  int64 `json:"cache_read"`
	CacheWrite int64 `json:"cache_write,omitempty"`
	Output     int64 `json:"output"`
}

type CostBreakdown struct {
	FreshInputMicros int64 `json:"fresh_input_micros"`
	CacheReadMicros  int64 `json:"cache_read_micros"`
	CacheWriteMicros int64 `json:"cache_write_micros,omitempty"`
	OutputMicros     int64 `json:"output_micros"`
	TotalMicros      int64 `json:"total_micros"`
}

type ProfileRef struct {
	Provider      string     `json:"provider"`
	SKU           string     `json:"sku"`
	Model         string     `json:"model"`
	Tier          string     `json:"tier,omitempty"`
	Currency      string     `json:"currency"`
	EffectiveFrom *time.Time `json:"effective_from,omitempty"`
}

type CatalogRef struct {
	Source  CatalogSource `json:"source"`
	Version string        `json:"version,omitempty"`
}

type MissingProfile struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model"`
	Count    int    `json:"count"`
	Sessions int    `json:"sessions,omitempty"`
	Turns    int    `json:"turns,omitempty"`
}

type BasisPoints int64

func NewStore(cacheDir, overridePath string) Store {
	return Store{
		cacheDir:     cacheDir,
		overridePath: overridePath,
		remoteURL:    DefaultRemoteURL(),
		client:       &http.Client{Timeout: updateTimeout},
	}
}

func DefaultStore(overridePath string) (Store, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return Store{}, fmt.Errorf("find user cache dir: %w", err)
	}
	if overridePath == "" {
		overridePath, err = DefaultOverridePath()
		if err != nil {
			return Store{}, err
		}
	}
	return NewStore(filepath.Join(dir, "tokdoctor", "pricing"), overridePath), nil
}

func DefaultRemoteURL() string {
	return defaultRemoteCatalogURL
}

func DefaultOverridePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find user config dir: %w", err)
	}
	return filepath.Join(dir, "tokdoctor", "pricing-overrides.json"), nil
}

func DecodeCatalog(r io.Reader) (Catalog, error) {
	var catalog Catalog
	if err := json.NewDecoder(r).Decode(&catalog); err != nil {
		return Catalog{}, fmt.Errorf("decode pricing catalog: %w", err)
	}
	if err := catalog.Validate(); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

func EmbeddedCatalog() (Catalog, error) {
	f, err := catalogFS.Open("embedded_catalog.json")
	if err != nil {
		return Catalog{}, fmt.Errorf("open embedded pricing catalog: %w", err)
	}
	defer f.Close()
	return DecodeCatalog(f)
}

func DefaultCatalog() (Catalog, error) {
	active, err := DefaultActiveCatalog("")
	if err != nil {
		return Catalog{}, err
	}
	return active.Catalog, nil
}

func DefaultActiveCatalog(overridePath string) (ActiveCatalog, error) {
	store, err := DefaultStore(overridePath)
	if err != nil {
		return ActiveCatalog{}, err
	}
	return store.Active()
}

func (s Store) Active() (ActiveCatalog, error) {
	embedded, err := EmbeddedCatalog()
	if err != nil {
		return ActiveCatalog{}, err
	}
	setCatalogSource(&embedded, SourceEmbedded)

	active := embedded
	activeSource := SourceEmbedded
	meta := CatalogMeta{Version: embedded.Version, URL: s.remoteURL}
	if cached, cachedMeta, err := s.readCache(); err == nil {
		setCatalogSource(&cached, SourceCache)
		active = mergeCatalogs(embedded, cached)
		activeSource = SourceCache
		meta = cachedMeta
	}

	if s.overridePath != "" {
		override, err := readCatalogFile(s.overridePath)
		if err != nil {
			if !os.IsNotExist(err) {
				return ActiveCatalog{}, fmt.Errorf("read pricing override: %w", err)
			}
		} else {
			setCatalogSource(&override, SourceOverride)
			active = mergeCatalogs(active, override)
			activeSource = SourceOverride
			meta.Version = active.Version
		}
	}
	return ActiveCatalog{Catalog: active, Source: activeSource, Meta: meta}, nil
}

func (s Store) Status() (Status, error) {
	active, err := s.Active()
	if err != nil {
		return Status{}, err
	}
	return Status{
		Source:      active.Source,
		Version:     active.Catalog.Version,
		GeneratedAt: active.Catalog.GeneratedAt,
		FetchedAt:   active.Meta.FetchedAt,
		Profiles:    len(active.Catalog.Profiles),
		RemoteURL:   s.remoteURL,
		ETag:        active.Meta.ETag,
	}, nil
}

func (s Store) List() (ActiveCatalog, error) {
	return s.Active()
}

func (s Store) Show(modelName string) (ActiveCatalog, PricingProfile, bool, error) {
	active, err := s.Active()
	if err != nil {
		return ActiveCatalog{}, PricingProfile{}, false, err
	}
	profile, ok := active.Catalog.Resolve(modelName, nil)
	return active, profile, ok, nil
}

func (s Store) AddOverride(profile PricingProfile, replace bool) (PricingProfile, error) {
	profile.CatalogSource = ""
	if err := profile.Validate(); err != nil {
		return PricingProfile{}, err
	}
	catalog, err := s.readOverride()
	if err != nil {
		return PricingProfile{}, err
	}
	if catalog.Version == "" {
		catalog.Version = "user-overrides"
	}
	if catalog.HasProviderProfile(profile.Provider, profile.SKU, profile.Model) {
		if !replace {
			return PricingProfile{}, fmt.Errorf("pricing override already exists for %s/%s", profile.Provider, profile.SKU)
		}
		catalog.Profiles = replaceProviderModel(catalog.Profiles, profile)
	} else {
		catalog.Profiles = append(catalog.Profiles, profile)
	}
	sortProfiles(catalog.Profiles)
	data, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return PricingProfile{}, fmt.Errorf("encode pricing override: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.overridePath), 0o755); err != nil {
		return PricingProfile{}, fmt.Errorf("create pricing override dir: %w", err)
	}
	if err := atomicWritePerm(s.overridePath, append(data, '\n'), 0o600); err != nil {
		return PricingProfile{}, err
	}
	return profile, nil
}

func (s Store) readOverride() (Catalog, error) {
	if s.overridePath == "" {
		return Catalog{Version: "user-overrides"}, nil
	}
	catalog, err := readCatalogFile(s.overridePath)
	if os.IsNotExist(err) {
		return Catalog{Version: "user-overrides"}, nil
	}
	if err != nil {
		return Catalog{}, fmt.Errorf("read pricing override: %w", err)
	}
	return catalog, nil
}

func (s Store) Update() (UpdateResult, error) {
	if err := validateRemoteURL(s.remoteURL); err != nil {
		return s.updateFailed(err)
	}
	meta, _ := s.readMeta()
	req, err := http.NewRequest(http.MethodGet, s.remoteURL, nil)
	if err != nil {
		return UpdateResult{}, fmt.Errorf("create pricing request: %w", err)
	}
	if meta.ETag != "" {
		req.Header.Set("If-None-Match", meta.ETag)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return s.updateFailed(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		active, err := s.Active()
		if err != nil {
			return UpdateResult{}, err
		}
		return UpdateResult{Updated: false, Source: active.Source, Version: active.Catalog.Version, Profiles: len(active.Catalog.Profiles), FetchedAt: active.Meta.FetchedAt}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return s.updateFailed(fmt.Errorf("pricing catalog http status %s", resp.Status))
	}
	limited := io.LimitReader(resp.Body, maxCatalogBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return s.updateFailed(fmt.Errorf("read pricing catalog: %w", err))
	}
	if len(data) > maxCatalogBytes {
		return s.updateFailed(errors.New("pricing catalog exceeds size limit"))
	}
	catalog, err := DecodeCatalog(strings.NewReader(string(data)))
	if err != nil {
		return s.updateFailed(err)
	}
	now := time.Now().UTC()
	newMeta := CatalogMeta{Version: catalog.Version, FetchedAt: &now, ETag: resp.Header.Get("ETag"), URL: s.remoteURL}
	if err := s.writeCache(data, newMeta); err != nil {
		return s.updateFailed(err)
	}
	return UpdateResult{Updated: true, Source: SourceCache, Version: catalog.Version, Profiles: len(catalog.Profiles), FetchedAt: &now}, nil
}

func (s Store) updateFailed(cause error) (UpdateResult, error) {
	if cached, meta, err := s.readCache(); err == nil {
		return UpdateResult{Updated: false, Source: SourceCache, Version: cached.Version, Profiles: len(cached.Profiles), FetchedAt: meta.FetchedAt, UsingCache: true}, fmt.Errorf("update pricing catalog: %w", cause)
	}
	return UpdateResult{}, fmt.Errorf("update pricing catalog: %w", cause)
}

func validateRemoteURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse pricing catalog URL: %w", err)
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme == "http" && (parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "localhost" || parsed.Hostname() == "::1") {
		return nil
	}
	return fmt.Errorf("pricing catalog URL must use HTTPS")
}

func (c Catalog) Resolve(modelName string, at *time.Time) (PricingProfile, bool) {
	var matches []PricingProfile
	for _, profile := range c.Profiles {
		if !profile.matches(modelName) {
			continue
		}
		if at != nil && profile.EffectiveFrom != nil && profile.EffectiveFrom.After(*at) {
			continue
		}
		matches = append(matches, profile)
	}
	if len(matches) == 0 {
		return PricingProfile{}, false
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return profileAfter(matches[i], matches[j])
	})
	return matches[0], true
}

func (c Catalog) HasProviderProfile(provider, sku, modelName string) bool {
	for _, profile := range c.Profiles {
		if !strings.EqualFold(profile.Provider, provider) {
			continue
		}
		if strings.EqualFold(profile.SKU, sku) || profile.matches(modelName) {
			return true
		}
	}
	return false
}

func (c Catalog) Validate() error {
	if c.Version == "" {
		return errors.New("pricing catalog version is required")
	}
	seen := map[string]seenAlias{}
	for i, profile := range c.Profiles {
		if err := profile.Validate(); err != nil {
			return fmt.Errorf("pricing profile %d: %w", i, err)
		}
		for _, alias := range profile.allAliases() {
			key := strings.ToLower(profile.Provider + "/" + alias)
			date := effectiveKey(profile.EffectiveFrom)
			if prev, ok := seen[key]; ok {
				if prev.sku != profile.SKU || prev.dates[date] {
					return fmt.Errorf("duplicate pricing alias %q for provider %s", alias, profile.Provider)
				}
				prev.dates[date] = true
				seen[key] = prev
				continue
			}
			seen[key] = seenAlias{sku: profile.SKU, dates: map[string]bool{date: true}}
		}
	}
	return nil
}

type seenAlias struct {
	sku   string
	dates map[string]bool
}

func (p PricingProfile) Validate() error {
	if p.Provider == "" {
		return errors.New("provider is required")
	}
	if p.SKU == "" {
		return errors.New("sku is required")
	}
	if p.Currency == "" {
		return errors.New("currency is required")
	}
	if len(p.allAliases()) == 0 {
		return errors.New("at least one model or alias is required")
	}
	if p.Rates.InputMicrosPerMillion < 0 || p.Rates.CachedInputMicrosPerMillion < 0 || p.Rates.CacheReadMicrosPerMillion < 0 || p.Rates.CacheWriteMicrosPerMillion < 0 || p.Rates.OutputMicrosPerMillion < 0 {
		return errors.New("rates must be non-negative")
	}
	for i, tier := range p.Tiers {
		if tier.Name == "" {
			return fmt.Errorf("tier %d name is required", i)
		}
		if tier.UpToInputTokens != nil && *tier.UpToInputTokens < 0 {
			return fmt.Errorf("tier %d up_to_input_tokens must be non-negative", i)
		}
		if tier.Rates.InputMicrosPerMillion < 0 || tier.Rates.CachedInputMicrosPerMillion < 0 || tier.Rates.CacheReadMicrosPerMillion < 0 || tier.Rates.CacheWriteMicrosPerMillion < 0 || tier.Rates.OutputMicrosPerMillion < 0 {
			return fmt.Errorf("tier %d rates must be non-negative", i)
		}
	}
	for i, source := range p.Sources {
		if source.URL == "" {
			return fmt.Errorf("source %d url is required", i)
		}
		parsed, err := url.Parse(source.URL)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
			return fmt.Errorf("source %d url is invalid", i)
		}
	}
	return nil
}

func effectiveKey(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339Nano)
}

func EstimateSession(session model.Session, active ActiveCatalog) CostResult {
	result := CostResult{
		SessionID:  session.ID,
		Model:      session.Model,
		Source:     session.Source,
		Status:     CostUnavailable,
		Provenance: CostUnknown,
		Catalog:    CatalogRef{Source: active.Source, Version: active.Catalog.Version},
	}
	if len(session.Turns) == 0 {
		turn := estimateUsage(session.Model, nil, session.Usage, active.Catalog)
		if turn.Status != CostAvailable {
			result.Unavailable = turn.Unavailable
			result.Missing = missingFromTurns([]TurnCost{turn})
			return result
		}
		result.Status = CostAvailable
		result.Provenance = CostEstimatedAPI
		result.Currency = turn.Currency
		result.Usage = turn.Usage
		result.Cost = turn.Cost
		result.Pricing = turn.Pricing
		result.CacheHitPct = cacheHitBasisPoints(*result.Usage)
		return result
	}

	var unavailable []string
	var totalUsage BillableTokens
	var totalCost CostBreakdown
	for _, turn := range session.Turns {
		turnCost := estimateUsage(turnModel(turn.Model, session.Model), turn.Timestamp, turn.Usage, active.Catalog)
		turnCost.ID = turn.ID
		turnCost.Sequence = turn.Sequence
		turnCost.Model = turnModel(turn.Model, session.Model)
		result.Turns = append(result.Turns, turnCost)
		if turnCost.Status != CostAvailable {
			if !strings.Contains(turnCost.Unavailable, "pricing profile unavailable") {
				unavailable = append(unavailable, turnUnavailableLabel(turnCost))
			}
			continue
		}
		if result.Currency == "" {
			result.Currency = turnCost.Currency
		}
		if result.Currency != turnCost.Currency {
			unavailable = append(unavailable, fmt.Sprintf("turn %d has currency %s", turnCost.Sequence, turnCost.Currency))
			continue
		}
		totalUsage = addBillableTokens(totalUsage, *turnCost.Usage)
		totalCost = addCosts(totalCost, *turnCost.Cost)
	}
	result.Missing = missingFromTurns(result.Turns)
	if len(result.Missing) > 0 {
		unavailable = append(unavailable, formatMissing(result.Missing)...)
	}
	if len(unavailable) > 0 {
		result.Unavailable = strings.Join(unavailable, "; ")
		return result
	}
	result.Status = CostAvailable
	result.Provenance = CostEstimatedAPI
	result.Usage = &totalUsage
	result.Cost = &totalCost
	result.CacheHitPct = cacheHitBasisPoints(totalUsage)
	return result
}

func EstimateCollection(sessions []model.Session, active ActiveCatalog, provider string) CostCollection {
	out := CostCollection{
		Status:     CostUnavailable,
		Provenance: CostUnknown,
		Provider:   provider,
		Catalog:    CatalogRef{Source: active.Source, Version: active.Catalog.Version},
	}
	type key struct {
		provider string
		source   string
	}
	groups := map[key]*ProviderCost{}
	models := map[key]map[string]*ModelCost{}
	sessionSeen := map[key]map[string]bool{}
	var missingTurns []TurnCost
	for _, session := range sessions {
		for _, turn := range turnsForCost(session) {
			turnCost := estimateUsage(turnModel(turn.Model, session.Model), turn.Timestamp, turn.Usage, active.Catalog)
			if turnCost.Status != CostAvailable {
				missingProvider, _ := splitProviderModel(turnCost.Model)
				if provider == "" || strings.EqualFold(missingProvider, provider) {
					missingTurns = append(missingTurns, turnCost)
				}
				continue
			}
			if provider != "" && !strings.EqualFold(turnCost.Pricing.Provider, provider) {
				continue
			}
			k := key{provider: turnCost.Pricing.Provider, source: session.Source}
			group := groups[k]
			if group == nil {
				group = &ProviderCost{Provider: k.provider, Source: k.source}
				groups[k] = group
				models[k] = map[string]*ModelCost{}
				sessionSeen[k] = map[string]bool{}
			}
			group.ModelCalls++
			sessionSeen[k][session.ID] = true
			addAvailableCost(&group.Usage, &group.Cost, *turnCost.Usage, *turnCost.Cost)
			mk := turnCost.Pricing.SKU + "\x00" + turnCost.Pricing.Model
			modelGroup := models[k][mk]
			if modelGroup == nil {
				modelGroup = &ModelCost{Model: turnCost.Pricing.Model, SKU: turnCost.Pricing.SKU, Currency: turnCost.Currency}
				models[k][mk] = modelGroup
			}
			modelGroup.ModelCalls++
			addAvailableCost(&modelGroup.Usage, &modelGroup.Cost, *turnCost.Usage, *turnCost.Cost)
			group.Currency = turnCost.Currency
		}
	}
	for k, group := range groups {
		group.Sessions = len(sessionSeen[k])
		for _, m := range models[k] {
			group.Models = append(group.Models, *m)
		}
		sort.SliceStable(group.Models, func(i, j int) bool {
			return group.Models[i].SKU < group.Models[j].SKU
		})
		out.Groups = append(out.Groups, *group)
	}
	sort.SliceStable(out.Groups, func(i, j int) bool {
		if out.Groups[i].Provider != out.Groups[j].Provider {
			return out.Groups[i].Provider < out.Groups[j].Provider
		}
		return out.Groups[i].Source < out.Groups[j].Source
	})
	out.Missing = missingFromTurns(missingTurns)
	if len(out.Groups) > 0 {
		out.Status = CostAvailable
		out.Provenance = CostEstimatedAPI
	}
	if provider == "" {
		out.Overlap = "possible overlap across sources; grand total omitted"
	}
	if len(out.Groups) == 0 {
		if provider == "" {
			out.Unavailable = "no resolvable cost data"
		} else {
			out.Unavailable = fmt.Sprintf("no resolvable cost data for provider %s", provider)
		}
	}
	return out
}

func MissingProfiles(sessions []model.Session, active ActiveCatalog) []MissingProfile {
	type stats struct {
		provider string
		model    string
		sessions map[string]bool
		turns    int
	}
	counts := map[string]*stats{}
	for _, session := range sessions {
		for _, turn := range turnsForCost(session) {
			modelName := turnModel(turn.Model, session.Model)
			if _, ok := active.Catalog.Resolve(modelName, turn.Timestamp); ok {
				continue
			}
			provider, modelOnly := splitProviderModel(modelName)
			key := strings.ToLower(provider + "/" + modelOnly)
			s := counts[key]
			if s == nil {
				s = &stats{provider: provider, model: modelOnly, sessions: map[string]bool{}}
				counts[key] = s
			}
			s.sessions[session.ID] = true
			s.turns++
		}
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]MissingProfile, 0, len(keys))
	for _, key := range keys {
		s := counts[key]
		out = append(out, MissingProfile{Provider: s.provider, Model: s.model, Count: s.turns, Sessions: len(s.sessions), Turns: s.turns})
	}
	return out
}

func estimateUsage(modelName string, at *time.Time, usage model.Usage, catalog Catalog) TurnCost {
	out := TurnCost{
		Model:      modelName,
		Status:     CostUnavailable,
		Provenance: CostUnknown,
	}
	profile, ok := catalog.Resolve(modelName, at)
	if !ok {
		out.Unavailable = fmt.Sprintf("pricing profile unavailable for model %q", modelName)
		return out
	}
	tokens, ok, reason := billableTokens(usage)
	if !ok {
		out.Unavailable = reason
		return out
	}
	rates, tierName, ok := profile.ratesForInput(tokens.TotalInput())
	if !ok {
		out.Unavailable = "pricing tier unavailable for turn input"
		return out
	}
	if rates.CacheWriteMicrosPerMillion > 0 && !usage.Billable.CacheWrite.Available() {
		out.Unavailable = "billable cache write unavailable"
		return out
	}
	cost := CostBreakdown{
		FreshInputMicros: costMicros(tokens.FreshInput, rates.InputMicrosPerMillion),
		CacheReadMicros:  costMicros(tokens.CacheRead, cacheReadRate(rates)),
		CacheWriteMicros: costMicros(tokens.CacheWrite, rates.CacheWriteMicrosPerMillion),
		OutputMicros:     costMicros(tokens.Output, rates.OutputMicrosPerMillion),
	}
	cost.TotalMicros = cost.FreshInputMicros + cost.CacheReadMicros + cost.CacheWriteMicros + cost.OutputMicros
	out.Status = CostAvailable
	out.Provenance = CostEstimatedAPI
	out.Currency = profile.Currency
	out.Usage = &tokens
	out.Cost = &cost
	out.Pricing = profile.ref(modelName, tierName)
	return out
}

func billableTokens(usage model.Usage) (BillableTokens, bool, string) {
	if usage.Billable == nil {
		return BillableTokens{}, false, "billable usage unavailable"
	}
	input, ok := measurementValue(usage.Billable.Input)
	if !ok {
		return BillableTokens{}, false, "billable input unavailable"
	}
	cacheRead, ok := measurementValue(usage.Billable.CacheRead)
	if !ok {
		return BillableTokens{}, false, "billable cache read unavailable"
	}
	output, ok := measurementValue(usage.Billable.Output)
	if !ok {
		return BillableTokens{}, false, "billable output unavailable"
	}
	cacheWrite := usage.Billable.CacheWrite.ValueOrZero()
	freshInput := input - cacheRead - cacheWrite
	if freshInput < 0 {
		return BillableTokens{}, false, "billable cache tokens exceed input"
	}
	return BillableTokens{
		FreshInput: freshInput,
		CacheRead:  cacheRead,
		CacheWrite: cacheWrite,
		Output:     output,
	}, true, ""
}

func (t BillableTokens) TotalInput() int64 {
	return t.FreshInput + t.CacheRead + t.CacheWrite
}

func (s Store) readCache() (Catalog, CatalogMeta, error) {
	catalog, err := readCatalogFile(s.catalogPath())
	if err != nil {
		return Catalog{}, CatalogMeta{}, err
	}
	meta, _ := s.readMeta()
	if meta.Version == "" {
		meta.Version = catalog.Version
	}
	return catalog, meta, nil
}

func (s Store) readMeta() (CatalogMeta, error) {
	f, err := os.Open(s.metaPath())
	if err != nil {
		return CatalogMeta{}, err
	}
	defer f.Close()
	var meta CatalogMeta
	if err := json.NewDecoder(f).Decode(&meta); err != nil {
		return CatalogMeta{}, err
	}
	return meta, nil
}

func (s Store) writeCache(data []byte, meta CatalogMeta) error {
	if err := os.MkdirAll(s.cacheDir, 0o755); err != nil {
		return fmt.Errorf("create pricing cache dir: %w", err)
	}
	if err := atomicWrite(s.catalogPath(), data); err != nil {
		return err
	}
	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("encode pricing metadata: %w", err)
	}
	return atomicWrite(s.metaPath(), append(metaData, '\n'))
}

func atomicWrite(path string, data []byte) error {
	return atomicWritePerm(path, data, 0o600)
}

func atomicWritePerm(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("set temp file permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("replace %s atomically: %w", path, err)
	}
	return nil
}

func (s Store) catalogPath() string {
	return filepath.Join(s.cacheDir, "catalog.json")
}

func (s Store) metaPath() string {
	return filepath.Join(s.cacheDir, "catalog.meta.json")
}

func readCatalogFile(path string) (Catalog, error) {
	f, err := os.Open(path)
	if err != nil {
		return Catalog{}, err
	}
	defer f.Close()
	return DecodeCatalog(io.LimitReader(f, maxCatalogBytes+1))
}

func mergeCatalogs(base, higher Catalog) Catalog {
	out := Catalog{Version: higher.Version, GeneratedAt: higher.GeneratedAt, Profiles: append([]PricingProfile{}, higher.Profiles...)}
	seen := map[string]bool{}
	for _, profile := range higher.Profiles {
		for _, alias := range profile.allAliases() {
			seen[strings.ToLower(profile.Provider+"/"+alias)] = true
		}
	}
	for _, profile := range base.Profiles {
		replaced := false
		for _, alias := range profile.allAliases() {
			if seen[strings.ToLower(profile.Provider+"/"+alias)] {
				replaced = true
				break
			}
		}
		if !replaced {
			out.Profiles = append(out.Profiles, profile)
		}
	}
	sortProfiles(out.Profiles)
	return out
}

func setCatalogSource(catalog *Catalog, source CatalogSource) {
	for i := range catalog.Profiles {
		catalog.Profiles[i].CatalogSource = source
	}
}

func sortProfiles(profiles []PricingProfile) {
	sort.SliceStable(profiles, func(i, j int) bool {
		if !strings.EqualFold(profiles[i].Provider, profiles[j].Provider) {
			return strings.ToLower(profiles[i].Provider) < strings.ToLower(profiles[j].Provider)
		}
		return strings.ToLower(profiles[i].SKU) < strings.ToLower(profiles[j].SKU)
	})
}

func replaceProviderModel(profiles []PricingProfile, profile PricingProfile) []PricingProfile {
	out := profiles[:0]
	for _, existing := range profiles {
		if strings.EqualFold(existing.Provider, profile.Provider) && (strings.EqualFold(existing.SKU, profile.SKU) || existing.matches(profile.Model)) {
			continue
		}
		out = append(out, existing)
	}
	return append(out, profile)
}

func profileAfter(a, b PricingProfile) bool {
	if sourceRank(a.CatalogSource) != sourceRank(b.CatalogSource) {
		return sourceRank(a.CatalogSource) > sourceRank(b.CatalogSource)
	}
	if a.EffectiveFrom == nil && b.EffectiveFrom != nil {
		return false
	}
	if a.EffectiveFrom != nil && b.EffectiveFrom == nil {
		return true
	}
	if a.EffectiveFrom != nil && b.EffectiveFrom != nil && !a.EffectiveFrom.Equal(*b.EffectiveFrom) {
		return a.EffectiveFrom.After(*b.EffectiveFrom)
	}
	return a.SKU < b.SKU
}

func sourceRank(source CatalogSource) int {
	switch source {
	case SourceOverride:
		return 3
	case SourceCache:
		return 2
	case SourceEmbedded:
		return 1
	default:
		return 0
	}
}

func (p PricingProfile) matches(modelName string) bool {
	for _, alias := range p.allAliases() {
		if strings.EqualFold(alias, modelName) {
			return true
		}
	}
	return false
}

func (p PricingProfile) allAliases() []string {
	seen := map[string]bool{}
	var aliases []string
	add := func(value string) {
		if value == "" {
			return
		}
		key := strings.ToLower(value)
		if seen[key] {
			return
		}
		seen[key] = true
		aliases = append(aliases, value)
	}
	add(p.Model)
	add(p.SKU)
	for _, alias := range p.Aliases {
		add(alias)
	}
	return aliases
}

func (p PricingProfile) ref(modelName, tier string) *ProfileRef {
	return &ProfileRef{
		Provider:      p.Provider,
		SKU:           p.SKU,
		Model:         modelName,
		Tier:          tier,
		Currency:      p.Currency,
		EffectiveFrom: p.EffectiveFrom,
	}
}

func (p PricingProfile) ratesForInput(input int64) (Rates, string, bool) {
	if len(p.Tiers) == 0 {
		return p.Rates, "", true
	}
	for _, tier := range p.Tiers {
		if tier.UpToInputTokens == nil || input <= *tier.UpToInputTokens {
			return tier.Rates, tier.Name, true
		}
	}
	return Rates{}, "", false
}

func cacheReadRate(rates Rates) int64 {
	if rates.CacheReadMicrosPerMillion != 0 {
		return rates.CacheReadMicrosPerMillion
	}
	return rates.CachedInputMicrosPerMillion
}

func measurementValue(m model.Measurement) (int64, bool) {
	if !m.Available() {
		return 0, false
	}
	return m.ValueOrZero(), true
}

func costMicros(tokens, rateMicrosPerMillion int64) int64 {
	return tokens * rateMicrosPerMillion / 1_000_000
}

func turnModel(turn, session string) string {
	if turn != "" {
		return turn
	}
	return session
}

func turnUnavailableLabel(turn TurnCost) string {
	if turn.Sequence > 0 {
		return fmt.Sprintf("turn %d: %s", turn.Sequence, turn.Unavailable)
	}
	return turn.Unavailable
}

func turnsForCost(session model.Session) []model.Turn {
	if len(session.Turns) > 0 {
		return session.Turns
	}
	return []model.Turn{{Sequence: 1, Model: session.Model, Usage: session.Usage, Timestamp: session.UpdatedAt}}
}

func addAvailableCost(usage **BillableTokens, cost **CostBreakdown, nextUsage BillableTokens, nextCost CostBreakdown) {
	if *usage == nil {
		*usage = &BillableTokens{}
	}
	if *cost == nil {
		*cost = &CostBreakdown{}
	}
	**usage = addBillableTokens(**usage, nextUsage)
	**cost = addCosts(**cost, nextCost)
}

func addBillableTokens(a, b BillableTokens) BillableTokens {
	return BillableTokens{
		FreshInput: a.FreshInput + b.FreshInput,
		CacheRead:  a.CacheRead + b.CacheRead,
		CacheWrite: a.CacheWrite + b.CacheWrite,
		Output:     a.Output + b.Output,
	}
}

func addCosts(a, b CostBreakdown) CostBreakdown {
	return CostBreakdown{
		FreshInputMicros: a.FreshInputMicros + b.FreshInputMicros,
		CacheReadMicros:  a.CacheReadMicros + b.CacheReadMicros,
		CacheWriteMicros: a.CacheWriteMicros + b.CacheWriteMicros,
		OutputMicros:     a.OutputMicros + b.OutputMicros,
		TotalMicros:      a.TotalMicros + b.TotalMicros,
	}
}

func cacheHitBasisPoints(tokens BillableTokens) *BasisPoints {
	totalInput := tokens.FreshInput + tokens.CacheRead + tokens.CacheWrite
	if totalInput == 0 {
		return nil
	}
	v := BasisPoints((tokens.CacheRead + tokens.CacheWrite) * 10_000 / totalInput)
	return &v
}

func missingFromTurns(turns []TurnCost) []MissingProfile {
	counts := map[string]int{}
	models := map[string]string{}
	for _, turn := range turns {
		if turn.Status == CostAvailable || !strings.Contains(turn.Unavailable, "pricing profile unavailable") {
			continue
		}
		model := turn.Model
		key := strings.ToLower(model)
		counts[key]++
		models[key] = model
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]MissingProfile, 0, len(keys))
	for _, key := range keys {
		modelName := models[key]
		if modelName == "" {
			modelName = "unknown model"
		}
		provider, modelName := splitProviderModel(modelName)
		out = append(out, MissingProfile{Provider: provider, Model: modelName, Count: counts[key], Turns: counts[key]})
	}
	return out
}

func formatMissing(missing []MissingProfile) []string {
	out := make([]string, 0, len(missing))
	for _, miss := range missing {
		label := miss.Model
		if miss.Provider != "" {
			label = miss.Provider + "/" + miss.Model
		}
		out = append(out, fmt.Sprintf("pricing unavailable for %s (%d turns)", label, miss.Count))
	}
	return out
}

func splitProviderModel(value string) (string, string) {
	provider, modelName, ok := strings.Cut(value, "/")
	if !ok || provider == "" || modelName == "" {
		return "", value
	}
	return provider, modelName
}

func DecimalMicros(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("rate is required")
	}
	if strings.HasPrefix(value, "-") {
		return 0, errors.New("rate must be non-negative")
	}
	whole, frac, ok := strings.Cut(value, ".")
	if !ok {
		whole, frac = value, ""
	}
	if whole == "" {
		whole = "0"
	}
	if len(frac) > 6 {
		return 0, errors.New("rate supports at most 6 decimal places")
	}
	if !digitsOnly(whole) || !digitsOnly(frac) {
		return 0, fmt.Errorf("invalid decimal rate %q", value)
	}
	wholeMicros, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse rate %q: %w", value, err)
	}
	for len(frac) < 6 {
		frac += "0"
	}
	fracMicros := int64(0)
	if frac != "" {
		fracMicros, err = strconv.ParseInt(frac, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse rate %q: %w", value, err)
		}
	}
	if wholeMicros > (1<<63-1-fracMicros)/1_000_000 {
		return 0, fmt.Errorf("rate %q overflows money representation", value)
	}
	return wholeMicros*1_000_000 + fracMicros, nil
}

func digitsOnly(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
