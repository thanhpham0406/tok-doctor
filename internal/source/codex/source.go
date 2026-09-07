package codex

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/thanhpham0406/tok-doctor/internal/model"
	"github.com/thanhpham0406/tok-doctor/internal/source"
)

const maxSessions = 50

type Source struct{}

func New() *Source {
	return &Source{}
}

func (s *Source) Name() string {
	return string(model.AgentCodex)
}

func (s *Source) DisplayName() string {
	return "Codex"
}

func (s *Source) Kind() source.Kind {
	return source.KindPath
}

func (s *Source) Detect(ctx context.Context, override source.Override) source.Detection {
	return s.detect(ctx, override)
}

func (s *Source) Test(ctx context.Context, override source.Override) source.Detection {
	return s.detect(ctx, override)
}

func (s *Source) detect(ctx context.Context, override source.Override) source.Detection {
	_ = ctx

	if override.Path != "" {
		return source.DetectConfiguredPath(s.Name(), s.DisplayName(), override.Path, override.Origin, s.capabilities(), hasCodexSessionData)
	}

	return source.DetectAutoPathSource(s.Name(), s.DisplayName(), installPaths(), sessionPaths(), executableNames(), s.capabilities(), hasCodexSessionData)
}

func (s *Source) EmptySession(ctx context.Context) model.Session {
	_ = ctx

	return model.Session{
		ID:    "scaffold",
		Agent: model.AgentCodex,
	}
}

func (s *Source) Sessions(ctx context.Context) ([]source.SessionRef, error) {
	_ = ctx
	paths, err := collectSessionFiles()
	if err != nil {
		return nil, err
	}
	type entry struct {
		path  string
		mtime int64
	}
	var entries []entry
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		entries = append(entries, entry{path: p, mtime: info.ModTime().UnixNano()})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].mtime > entries[j].mtime
	})
	if len(entries) > maxSessions {
		entries = entries[:maxSessions]
	}
	refs := make([]source.SessionRef, 0, len(entries))
	for _, e := range entries {
		refs = append(refs, source.SessionRef{
			ID:   filepath.Base(e.path),
			Path: e.path,
		})
	}
	return refs, nil
}

func (s *Source) Usage(ctx context.Context, refs []source.SessionRef) (model.Usage, error) {
	_ = ctx
	var snaps []UsageSnapshot
	for _, ref := range refs {
		snap, err := ParseSessionUsage(ref.Path)
		if err != nil {
			return model.Usage{}, err
		}
		if snap.Total == 0 && snap.Input == 0 && snap.Output == 0 {
			continue
		}
		snaps = append(snaps, snap)
	}
	return SumSnapshots(snaps), nil
}

func (s *Source) ReadSessions(ctx context.Context) ([]model.Session, error) {
	refs, err := s.Sessions(ctx)
	if err != nil {
		return nil, err
	}
	sessions := make([]model.Session, 0, len(refs))
	for _, ref := range refs {
		parsed, err := ParseSession(ref.Path)
		if err != nil {
			return nil, err
		}
		id := parsed.ID
		if id == "" {
			id = stableFileID(ref)
		}
		session := model.Session{
			ID:        id,
			Source:    s.Name(),
			Agent:     model.AgentCodex,
			StartedAt: parseTime(parsed.StartedAt),
			UpdatedAt: parseTime(parsed.UpdatedAt),
			Model:     parsed.Model,
			Usage:     parsed.Usage.ToModelUsage(),
		}
		if session.UpdatedAt == nil {
			session.UpdatedAt = fileModTime(ref.Path)
		}
		if session.Usage.Confidence != "" {
			session.Evidence = []model.Evidence{{Kind: "provider_usage", Source: "codex_rollout"}}
		}
		sessions = append(sessions, session)
	}
	sortSessions(sessions)
	return sessions, nil
}

func (s *Source) capabilities() *source.Capabilities {
	return &source.Capabilities{
		SessionDiscovery: true,
		TokenUsage:       true,
		ToolCalls:        true,
		ToolOutput:       true,
	}
}

func stableFileID(ref source.SessionRef) string {
	name := ref.ID
	if name == "" {
		name = filepath.Base(ref.Path)
	}
	return strings.TrimSuffix(name, filepath.Ext(name))
}

func parseTime(value string) *time.Time {
	if value == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil
	}
	return &t
}

func fileModTime(path string) *time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	t := info.ModTime()
	return &t
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
		return sessions[i].ID < sessions[j].ID
	})
}

func hasCodexSessionData(path string) bool {
	return source.HasFileWithSuffix(path, ".jsonl", 4)
}

func collectSessionFiles() ([]string, error) {
	var out []string
	for _, base := range sessionPaths() {
		err := filepath.WalkDir(base.Path, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				if d != nil && d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if filepath.Ext(path) == ".jsonl" {
				out = append(out, path)
			}
			return nil
		})
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
	}
	return out, nil
}
