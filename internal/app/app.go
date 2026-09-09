package app

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thanhpham0406/tok-doctor/internal/analyze"
	"github.com/thanhpham0406/tok-doctor/internal/config"
	"github.com/thanhpham0406/tok-doctor/internal/gateway"
	"github.com/thanhpham0406/tok-doctor/internal/model"
	"github.com/thanhpham0406/tok-doctor/internal/pricing"
	"github.com/thanhpham0406/tok-doctor/internal/source"
	"github.com/thanhpham0406/tok-doctor/internal/source/catalog"
	"github.com/thanhpham0406/tok-doctor/internal/source/codex"
)

type App struct {
	codex    *codex.Source
	analyzer *analyze.Analyzer
	sources  catalog.Registry
	config   config.Store
}

func New() *App {
	store, err := config.NewStore()
	if err != nil {
		store = config.NewStoreAt("")
	}
	return NewWithStore(store)
}

func NewWithStore(store config.Store) *App {
	return &App{
		codex:    codex.New(),
		analyzer: analyze.New(),
		sources:  catalog.Builtins(),
		config:   store,
	}
}

func (a *App) Doctor(ctx context.Context) (analyze.Result, error) {
	session := a.codex.EmptySession(ctx)
	return a.analyzer.Analyze(ctx, session), nil
}

type usageSource interface {
	Sessions(ctx context.Context) ([]source.SessionRef, error)
	Usage(ctx context.Context, refs []source.SessionRef) (model.Usage, error)
}

type sessionSource interface {
	ReadSessions(ctx context.Context) ([]model.Session, error)
}

func (a *App) Usage(ctx context.Context, name string) (model.UsageEntry, error) {
	name = normalizeName(name)
	detector, err := a.sources.Detector(name)
	if err != nil {
		return model.UsageEntry{}, err
	}
	src, ok := detector.(usageSource)
	if !ok {
		return model.UsageEntry{}, fmt.Errorf("usage not supported for %s", name)
	}
	refs, err := src.Sessions(ctx)
	if err != nil {
		return model.UsageEntry{}, fmt.Errorf("usage for %s: %w", name, err)
	}
	usage, err := src.Usage(ctx, refs)
	if err != nil {
		return model.UsageEntry{}, fmt.Errorf("usage for %s: %w", name, err)
	}
	return model.UsageEntry{Source: name, Usage: usage}, nil
}

func (a *App) UsageAll(ctx context.Context) (model.UsageResult, error) {
	var result model.UsageResult
	for _, detector := range a.sources.Detectors() {
		src, ok := detector.(usageSource)
		if !ok {
			continue
		}
		refs, err := src.Sessions(ctx)
		if err != nil {
			return model.UsageResult{}, fmt.Errorf("usage for %s: %w", detector.Name(), err)
		}
		usage, err := src.Usage(ctx, refs)
		if err != nil {
			return model.UsageResult{}, fmt.Errorf("usage for %s: %w", detector.Name(), err)
		}
		result.Sources = append(result.Sources, model.UsageEntry{Source: detector.Name(), Usage: usage})
	}
	return result, nil
}

func (a *App) Sessions(ctx context.Context, name string) (model.SessionsResult, error) {
	name = normalizeName(name)
	detector, err := a.sources.Detector(name)
	if err != nil {
		return model.SessionsResult{}, err
	}
	src, ok := detector.(sessionSource)
	if !ok {
		return model.SessionsResult{}, fmt.Errorf("sessions not supported for %s", name)
	}
	sessions, err := src.ReadSessions(ctx)
	if err != nil {
		return model.SessionsResult{}, fmt.Errorf("sessions for %s: %w", name, err)
	}
	sessions = sessionsWithAuthoritativeUsage(sessions)
	return model.SessionsResult{Sessions: sessions}, nil
}

func (a *App) SessionsAll(ctx context.Context) (model.SessionsResult, error) {
	var result model.SessionsResult
	for _, detector := range a.sources.Detectors() {
		src, ok := detector.(sessionSource)
		if !ok {
			continue
		}
		sessions, err := src.ReadSessions(ctx)
		if err != nil {
			return model.SessionsResult{}, fmt.Errorf("sessions for %s: %w", detector.Name(), err)
		}
		result.Sessions = append(result.Sessions, sessions...)
	}
	result.Sessions = sessionsWithAuthoritativeUsage(result.Sessions)
	sortSessions(result.Sessions)
	return result, nil
}

func sessionsWithAuthoritativeUsage(sessions []model.Session) []model.Session {
	filtered := sessions[:0]
	for _, session := range sessions {
		if session.HasAuthoritativeUsage() {
			filtered = append(filtered, session)
		}
	}
	return filtered
}

var (
	ErrInspectSessionNotFound  = errors.New("session not found")
	ErrInspectAmbiguousSession = errors.New("ambiguous session id")
)

func (a *App) Inspect(ctx context.Context, id string) (model.Session, error) {
	sessions, err := a.SessionsAll(ctx)
	if err != nil {
		return model.Session{}, err
	}
	return pickSessionByID(sessions.Sessions, id)
}

func (a *App) Cost(ctx context.Context, id string) (pricing.CostResult, error) {
	session, err := a.Inspect(ctx, id)
	if err != nil {
		return pricing.CostResult{}, err
	}
	active, err := a.activePricingCatalog()
	if err != nil {
		return pricing.CostResult{}, err
	}
	return pricing.EstimateSession(session, active), nil
}

func (a *App) CostAll(ctx context.Context) (pricing.CostCollection, error) {
	sessions, err := a.SessionsAll(ctx)
	if err != nil {
		return pricing.CostCollection{}, err
	}
	active, err := a.activePricingCatalog()
	if err != nil {
		return pricing.CostCollection{}, err
	}
	return pricing.EstimateCollection(sessions.Sessions, active, ""), nil
}

func (a *App) CostProvider(ctx context.Context, provider string) (pricing.CostCollection, error) {
	sessions, err := a.SessionsAll(ctx)
	if err != nil {
		return pricing.CostCollection{}, err
	}
	active, err := a.activePricingCatalog()
	if err != nil {
		return pricing.CostCollection{}, err
	}
	return pricing.EstimateCollection(sessions.Sessions, active, provider), nil
}

func (a *App) PricingStatus() (pricing.Status, error) {
	store, err := a.pricingStore()
	if err != nil {
		return pricing.Status{}, err
	}
	return store.Status()
}

func (a *App) PricingList() (pricing.ActiveCatalog, error) {
	store, err := a.pricingStore()
	if err != nil {
		return pricing.ActiveCatalog{}, err
	}
	return store.List()
}

func (a *App) PricingShow(modelName string) (pricing.ActiveCatalog, pricing.PricingProfile, bool, error) {
	store, err := a.pricingStore()
	if err != nil {
		return pricing.ActiveCatalog{}, pricing.PricingProfile{}, false, err
	}
	return store.Show(modelName)
}

func (a *App) PricingUpdate() (pricing.UpdateResult, error) {
	store, err := a.pricingStore()
	if err != nil {
		return pricing.UpdateResult{}, err
	}
	return store.Update()
}

func (a *App) PricingAdd(profile pricing.PricingProfile, replace bool) (pricing.PricingProfile, error) {
	store, err := a.pricingStore()
	if err != nil {
		return pricing.PricingProfile{}, err
	}
	return store.AddOverride(profile, replace)
}

func (a *App) PricingMissing(ctx context.Context) ([]pricing.MissingProfile, error) {
	sessions, err := a.SessionsAll(ctx)
	if err != nil {
		return nil, err
	}
	active, err := a.activePricingCatalog()
	if err != nil {
		return nil, err
	}
	return pricing.MissingProfiles(sessions.Sessions, active), nil
}

func (a *App) activePricingCatalog() (pricing.ActiveCatalog, error) {
	store, err := a.pricingStore()
	if err != nil {
		return pricing.ActiveCatalog{}, err
	}
	return store.Active()
}

func (a *App) pricingStore() (pricing.Store, error) {
	cfg, err := a.config.Load()
	if err != nil {
		return pricing.Store{}, err
	}
	return pricing.DefaultStore(cfg.Pricing.OverridePath)
}

func pickSessionByID(sessions []model.Session, id string) (model.Session, error) {
	if id == "" {
		return model.Session{}, fmt.Errorf("inspect session id: %w", ErrInspectSessionNotFound)
	}
	if len(sessions) == 0 {
		return model.Session{}, fmt.Errorf("inspect session %q: %w", id, ErrInspectSessionNotFound)
	}
	for _, sess := range sessions {
		if sess.ID == id {
			return sess, nil
		}
	}
	var matches []model.Session
	for _, sess := range sessions {
		if strings.HasPrefix(sess.ID, id) {
			matches = append(matches, sess)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return model.Session{}, fmt.Errorf("inspect session %q: %w", id, ErrInspectAmbiguousSession)
	}
	return model.Session{}, fmt.Errorf("inspect session %q: %w", id, ErrInspectSessionNotFound)
}

func (a *App) Sources(ctx context.Context) (source.ListResult, error) {
	cfg, err := a.config.Load()
	if err != nil {
		return source.ListResult{}, err
	}
	return a.sources.All(ctx, cfg), nil
}

func sortSessions(sessions []model.Session) {
	sort.SliceStable(sessions, func(i, j int) bool {
		left := sessions[i].UpdatedAt
		right := sessions[j].UpdatedAt
		if left != nil && right != nil && !left.Equal(*right) {
			return left.After(*right)
		}
		if left != nil && right == nil {
			return true
		}
		if left == nil && right != nil {
			return false
		}
		if sessions[i].Source != sessions[j].Source {
			return sessions[i].Source < sessions[j].Source
		}
		return sessions[i].ID < sessions[j].ID
	})
}

func (a *App) ShowSource(ctx context.Context, name string, override source.Override) (source.Detection, error) {
	cfg, err := a.config.Load()
	if err != nil {
		return source.Detection{}, err
	}
	name = normalizeName(name)
	if err := a.validateCLIOverride(name, override); err != nil {
		return source.Detection{}, err
	}
	return a.sources.Show(ctx, cfg, name, override)
}

func (a *App) TestSource(ctx context.Context, name string, override source.Override) (source.Detection, error) {
	cfg, err := a.config.Load()
	if err != nil {
		return source.Detection{}, err
	}
	name = normalizeName(name)
	if err := a.validateCLIOverride(name, override); err != nil {
		return source.Detection{}, err
	}
	return a.sources.Test(ctx, cfg, name, override)
}

func (a *App) SetSource(name string, override source.Override) error {
	name = normalizeName(name)
	kind, err := a.sources.Kind(name)
	if err != nil {
		return err
	}
	if err := validateOverride(kind, override); err != nil {
		return err
	}

	switch kind {
	case source.KindPath:
		return a.config.SetSource(name, config.Source{Path: override.Path})
	case source.KindEndpoint:
		return a.config.SetSource(name, config.Source{Endpoint: override.Endpoint})
	default:
		return fmt.Errorf("unsupported source kind %q", kind)
	}
}

func (a *App) ResetSource(name string) error {
	name = normalizeName(name)
	if !a.sources.ValidateName(name) {
		return fmt.Errorf("unknown source %q", name)
	}
	return a.config.ResetSource(name)
}

func validateOverride(kind source.Kind, override source.Override) error {
	switch kind {
	case source.KindPath:
		if override.Path == "" {
			return fmt.Errorf("path is required for this source")
		}
		if !filepath.IsAbs(override.Path) {
			return fmt.Errorf("path must be absolute")
		}
		if override.Endpoint != "" {
			return fmt.Errorf("endpoint is not supported for this source")
		}
	case source.KindEndpoint:
		if override.Endpoint == "" {
			return fmt.Errorf("endpoint is required for this source")
		}
		if _, err := url.ParseRequestURI(override.Endpoint); err != nil {
			return fmt.Errorf("invalid endpoint: %w", err)
		}
		if err := source.ValidateLocalEndpoint(override.Endpoint); err != nil {
			return err
		}
		if override.Path != "" {
			return fmt.Errorf("path is not supported for this source")
		}
	default:
		return fmt.Errorf("unsupported source kind %q", kind)
	}
	return nil
}

func (a *App) validateCLIOverride(name string, override source.Override) error {
	if override.Path == "" && override.Endpoint == "" {
		return nil
	}
	kind, err := a.sources.Kind(name)
	if err != nil {
		return err
	}
	return validateOverride(kind, override)
}

func normalizeName(name string) string {
	return strings.ToLower(name)
}

func (a *App) GatewayStart(ctx context.Context, profileName string) ([]gateway.StartResult, error) {
	cfg, err := a.config.Load()
	if err != nil {
		return nil, err
	}
	set := gateway.ValidateProfiles(cfg.Gateway.Profiles).FilterByName(profileName)
	if len(set.Errors) > 0 {
		errs := make([]string, 0, len(set.Errors))
		for _, e := range set.Errors {
			errs = append(errs, fmt.Sprintf("%s: %s", e.Name, e.Reason))
		}
		return nil, errors.New(strings.Join(errs, "; "))
	}
	manager, err := gateway.NewManager()
	if err != nil {
		return nil, err
	}
	return manager.Start(ctx, set.Profiles)
}

func (a *App) GatewayServe(ctx context.Context, profileName string) error {
	if profileName == "" {
		return errors.New("profile is required")
	}
	cfg, err := a.config.Load()
	if err != nil {
		return err
	}
	set := gateway.ValidateProfiles(cfg.Gateway.Profiles).FilterByName(profileName)
	if len(set.Errors) > 0 {
		errs := make([]string, 0, len(set.Errors))
		for _, e := range set.Errors {
			errs = append(errs, fmt.Sprintf("%s: %s", e.Name, e.Reason))
		}
		return errors.New(strings.Join(errs, "; "))
	}
	dir, err := gateway.DefaultDir()
	if err != nil {
		return err
	}
	recorder, err := gateway.NewFileRecorder(dir)
	if err != nil {
		return err
	}
	rt := gateway.NewRuntime(recorder)
	if err := rt.Start(ctx, set.Profiles); err != nil {
		_ = recorder.Close()
		return err
	}
	defer func() { _ = recorder.Close() }()
	defer func() { _ = rt.Shutdown(context.Background()) }()
	if err := rt.Wait(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func (a *App) GatewayStatus(profileName string) ([]gateway.StatusEntry, error) {
	cfg, err := a.config.Load()
	if err != nil {
		return nil, err
	}
	set := gateway.ValidateProfiles(cfg.Gateway.Profiles).FilterByName(profileName)
	if len(set.Errors) > 0 {
		errs := make([]string, 0, len(set.Errors))
		for _, e := range set.Errors {
			errs = append(errs, fmt.Sprintf("%s: %s", e.Name, e.Reason))
		}
		return nil, errors.New(strings.Join(errs, "; "))
	}
	dir, err := gateway.DefaultDir()
	if err != nil {
		return nil, err
	}
	recorder, err := gateway.NewFileRecorder(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = recorder.Close() }()
	rt := gateway.NewRuntime(recorder)
	return rt.Status(set.Profiles), nil
}

func (a *App) GatewayRequests(profileName string) ([]gateway.RequestSummary, error) {
	if err := requireProfile(profileName); err != nil {
		return nil, err
	}
	if err := gateway.ValidateCaptureProfile(profileName); err != nil {
		return nil, err
	}
	recorder, err := a.openRecorder()
	if err != nil {
		return nil, err
	}
	defer func() { _ = recorder.Close() }()
	exchanges, err := recorder.ListExchanges(profileName)
	if err != nil {
		return nil, err
	}
	summaries := make([]gateway.RequestSummary, 0, len(exchanges))
	for _, e := range exchanges {
		summaries = append(summaries, gateway.SummarizeRequest(e))
	}
	return summaries, nil
}

func (a *App) GatewayInspect(profileName, id string) (gateway.RequestSummary, error) {
	if err := requireProfile(profileName); err != nil {
		return gateway.RequestSummary{}, err
	}
	if err := gateway.ValidateCaptureProfile(profileName); err != nil {
		return gateway.RequestSummary{}, err
	}
	if id == "" {
		return gateway.RequestSummary{}, fmt.Errorf("exchange id is required")
	}
	recorder, err := a.openRecorder()
	if err != nil {
		return gateway.RequestSummary{}, err
	}
	defer func() { _ = recorder.Close() }()
	exchange, err := recorder.FindExchange(profileName, id)
	if err != nil {
		return gateway.RequestSummary{}, err
	}
	return gateway.SummarizeRequest(exchange), nil
}

func (a *App) GatewayChains(profileName string) ([]gateway.ChainSummary, error) {
	if err := requireProfile(profileName); err != nil {
		return nil, err
	}
	if err := gateway.ValidateCaptureProfile(profileName); err != nil {
		return nil, err
	}
	recorder, err := a.openRecorder()
	if err != nil {
		return nil, err
	}
	defer func() { _ = recorder.Close() }()
	exchanges, err := gateway.Replay(recorder, profileName)
	if err != nil {
		return nil, err
	}
	return gateway.AnalyzeChains(exchanges), nil
}

func (a *App) GatewayChain(profileName, id string) (gateway.ChainSummary, error) {
	chains, err := a.GatewayChains(profileName)
	if err != nil {
		return gateway.ChainSummary{}, err
	}
	return gateway.FindChain(chains, id)
}

func (a *App) openRecorder() (*gateway.FileRecorder, error) {
	dir, err := gateway.DefaultDir()
	if err != nil {
		return nil, err
	}
	return gateway.NewFileRecorder(dir)
}

func requireProfile(name string) error {
	if name == "" {
		return errors.New("--profile is required")
	}
	return nil
}

func (a *App) GatewayStop(ctx context.Context, profileName string) ([]gateway.StatusEntry, error) {
	cfg, err := a.config.Load()
	if err != nil {
		return nil, err
	}
	set := gateway.ValidateProfiles(cfg.Gateway.Profiles).FilterByName(profileName)
	if len(set.Errors) > 0 {
		errs := make([]string, 0, len(set.Errors))
		for _, e := range set.Errors {
			errs = append(errs, fmt.Sprintf("%s: %s", e.Name, e.Reason))
		}
		return nil, errors.New(strings.Join(errs, "; "))
	}
	manager, err := gateway.NewManager()
	if err != nil {
		return nil, err
	}
	return manager.Stop(ctx, set.Profiles)
}

type GatewaySetupOptions struct {
	Source   string
	Listen   string
	Protocol string
	Upstream string
	Provider string
}

func (a *App) GatewaySetup(ctx context.Context, opts GatewaySetupOptions) (gateway.Profile, bool, error) {
	sourceName := normalizeName(opts.Source)
	if sourceName == "" {
		sourceName = "codex"
	}
	if !a.sources.ValidateName(sourceName) {
		return gateway.Profile{}, false, fmt.Errorf("unknown source %q", sourceName)
	}
	cfg, err := a.config.Load()
	if err != nil {
		return gateway.Profile{}, false, err
	}
	if existing, ok := cfg.Gateway.Profiles[sourceName]; ok {
		profile := gateway.Profile{
			Name:        sourceName,
			Enabled:     existing.Enabled,
			Listen:      existing.Listen,
			Protocol:    existing.Protocol,
			Source:      existing.Source,
			Upstream:    existing.Upstream,
			ProviderTag: existing.ProviderTag,
		}
		return profile, false, nil
	}
	protocol := strings.TrimSpace(opts.Protocol)
	if protocol == "" {
		protocol = defaultGatewayProtocol(sourceName)
	}
	if protocol == "" {
		return gateway.Profile{}, false, fmt.Errorf("gateway setup for %s requires --protocol", sourceName)
	}
	upstream := strings.TrimSpace(opts.Upstream)
	if upstream == "" {
		upstream = cfg.Sources[sourceName].Endpoint
	}
	if upstream == "" {
		upstream = defaultGatewayUpstream(sourceName, protocol)
	}
	if upstream == "" {
		return gateway.Profile{}, false, fmt.Errorf("gateway setup for %s requires --upstream", sourceName)
	}
	listen := strings.TrimSpace(opts.Listen)
	if listen == "" {
		listen, err = gateway.FreeLoopbackAddress()
		if err != nil {
			return gateway.Profile{}, false, err
		}
	}
	raw := config.GatewayProfile{
		Enabled:     true,
		Listen:      listen,
		Protocol:    protocol,
		Source:      sourceName,
		Upstream:    upstream,
		ProviderTag: strings.TrimSpace(opts.Provider),
	}
	if cfg.Gateway.Profiles == nil {
		cfg.Gateway.Profiles = map[string]config.GatewayProfile{}
	}
	cfg.Gateway.Profiles[sourceName] = raw
	set := gateway.ValidateProfiles(cfg.Gateway.Profiles)
	if len(set.Errors) > 0 {
		return gateway.Profile{}, false, fmt.Errorf("%s: %s", set.Errors[0].Name, set.Errors[0].Reason)
	}
	if err := a.config.SetGatewayProfile(sourceName, raw); err != nil {
		return gateway.Profile{}, false, err
	}
	for _, profile := range set.Profiles {
		if profile.Name == sourceName {
			return profile, true, nil
		}
	}
	return gateway.Profile{}, false, fmt.Errorf("gateway profile %s was not created", sourceName)
}

func (a *App) GatewayRemove(ctx context.Context, profileName string, opts GatewayRemoveOptions) error {
	if profileName == "" {
		return errors.New("profile is required")
	}
	cfg, err := a.config.Load()
	if err != nil {
		return err
	}
	manager, err := gateway.NewManager()
	if err != nil {
		return err
	}
	if raw, ok := cfg.Gateway.Profiles[profileName]; ok {
		profile := gateway.Profile{
			Name:        profileName,
			Enabled:     raw.Enabled,
			Listen:      raw.Listen,
			Protocol:    raw.Protocol,
			Source:      raw.Source,
			Upstream:    raw.Upstream,
			ProviderTag: raw.ProviderTag,
		}
		if _, err := manager.Stop(ctx, []gateway.Profile{profile}); err != nil {
			return err
		}
	}
	removed := false
	if _, ok := cfg.Gateway.Profiles[profileName]; ok {
		if err := a.config.RemoveGatewayProfile(profileName); err != nil {
			return err
		}
		removed = true
	}
	manager.Store.Remove(profileName)
	if opts.Purge {
		dir, err := gateway.DefaultDir()
		if err != nil {
			return err
		}
		recorder, err := gateway.NewFileRecorder(dir)
		if err != nil {
			return err
		}
		_ = recorder.Close()
		if err := recorder.Purge(profileName); err != nil {
			return err
		}
	}
	if !removed && !opts.Purge {
		return fmt.Errorf("gateway profile %q not found", profileName)
	}
	return nil
}

type GatewayRemoveOptions struct {
	Purge bool
}

func defaultGatewayProtocol(sourceName string) string {
	switch sourceName {
	case "codex":
		return "openai_responses"
	case "claude":
		return "anthropic_messages"
	default:
		return ""
	}
}

func defaultGatewayUpstream(sourceName, protocol string) string {
	if sourceName == "codex" && protocol == "openai_responses" {
		return "https://api.openai.com"
	}
	return ""
}
