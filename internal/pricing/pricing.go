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
	"strings"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/model"
)

//go:embed catalog.json
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
	Provider      string      `json:"provider"`
	SKU           string      `json:"sku"`
	Model         string      `json:"model,omitempty"`
	Aliases       []string    `json:"aliases,omitempty"`
	Currency      string      `json:"currency"`
	EffectiveFrom *time.Time  `json:"effective_from,omitempty"`
	Rates         Rates       `json:"rates"`
	Sources       []SourceRef `json:"sources,omitempty"`
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
	return NewStore(filepath.Join(dir, "tokdoctor", "pricing"), overridePath), nil
}

func DefaultRemoteURL() string {
	return defaultRemoteCatalogURL
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
	f, err := catalogFS.Open("catalog.json")
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
	if s.overridePath != "" {
		catalog, err := readCatalogFile(s.overridePath)
		if err != nil {
			return ActiveCatalog{}, fmt.Errorf("read pricing override: %w", err)
		}
		return ActiveCatalog{Catalog: catalog, Source: SourceOverride, Meta: CatalogMeta{Version: catalog.Version}}, nil
	}
	if cached, meta, err := s.readCache(); err == nil {
		return ActiveCatalog{Catalog: cached, Source: SourceCache, Meta: meta}, nil
	}
	embedded, err := EmbeddedCatalog()
	if err != nil {
		return ActiveCatalog{}, err
	}
	return ActiveCatalog{Catalog: embedded, Source: SourceEmbedded, Meta: CatalogMeta{Version: embedded.Version, URL: s.remoteURL}}, nil
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
				if provider == "" || strings.EqualFold(providerHint(turnCost.Model), provider) {
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
	tokens, ok, reason := billableTokens(usage, profile)
	if !ok {
		out.Unavailable = reason
		return out
	}
	cost := CostBreakdown{
		FreshInputMicros: costMicros(tokens.FreshInput, profile.Rates.InputMicrosPerMillion),
		CacheReadMicros:  costMicros(tokens.CacheRead, cacheReadRate(profile)),
		CacheWriteMicros: costMicros(tokens.CacheWrite, profile.Rates.CacheWriteMicrosPerMillion),
		OutputMicros:     costMicros(tokens.Output, profile.Rates.OutputMicrosPerMillion),
	}
	cost.TotalMicros = cost.FreshInputMicros + cost.CacheReadMicros + cost.CacheWriteMicros + cost.OutputMicros
	out.Status = CostAvailable
	out.Provenance = CostEstimatedAPI
	out.Currency = profile.Currency
	out.Usage = &tokens
	out.Cost = &cost
	out.Pricing = profile.ref(modelName)
	return out
}

func billableTokens(usage model.Usage, profile PricingProfile) (BillableTokens, bool, string) {
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
	if profile.Rates.CacheWriteMicrosPerMillion > 0 && !usage.Billable.CacheWrite.Available() {
		return BillableTokens{}, false, "billable cache write unavailable"
	}
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
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
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

func profileAfter(a, b PricingProfile) bool {
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

func (p PricingProfile) ref(modelName string) *ProfileRef {
	return &ProfileRef{
		Provider:      p.Provider,
		SKU:           p.SKU,
		Model:         modelName,
		Currency:      p.Currency,
		EffectiveFrom: p.EffectiveFrom,
	}
}

func cacheReadRate(profile PricingProfile) int64 {
	if profile.Rates.CacheReadMicrosPerMillion != 0 {
		return profile.Rates.CacheReadMicrosPerMillion
	}
	return profile.Rates.CachedInputMicrosPerMillion
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
		out = append(out, MissingProfile{Provider: providerHint(modelName), Model: modelName, Count: counts[key]})
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

func providerHint(modelName string) string {
	name := strings.ToLower(modelName)
	switch {
	case strings.HasPrefix(name, "gpt-"), strings.HasPrefix(name, "o1"), strings.HasPrefix(name, "o3"), strings.HasPrefix(name, "o4"):
		return "openai"
	case strings.HasPrefix(name, "minimax-"):
		return "minimax"
	case strings.HasPrefix(name, "claude-"):
		return "anthropic"
	default:
		return ""
	}
}
