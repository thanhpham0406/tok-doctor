package claude

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
	return "claude"
}

func (s *Source) DisplayName() string {
	return "Claude"
}

func (s *Source) Kind() source.Kind {
	return source.KindPath
}

func (s *Source) Detect(ctx context.Context, override source.Override) source.Detection {
	_ = ctx
	if override.Path != "" {
		return source.DetectConfiguredPath(s.Name(), s.DisplayName(), override.Path, override.Origin, s.capabilities(), hasClaudeSessionData)
	}
	return source.DetectAutoPathSource(s.Name(), s.DisplayName(), installPaths(), sessionPaths(), executableNames(), s.capabilities(), hasClaudeSessionData)
}

func (s *Source) Test(ctx context.Context, override source.Override) source.Detection {
	return s.Detect(ctx, override)
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
		if !snap.HasPositiveUsage() {
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
		if !parsed.Usage.HasPositiveUsage() {
			continue
		}
		id := stableFileID(ref)
		turns := claudeTurns(id, parsed)
		session := model.Session{
			ID:        id,
			Source:    s.Name(),
			Agent:     model.AgentClaude,
			StartedAt: parseTime(parsed.StartedAt),
			UpdatedAt: parseTime(parsed.UpdatedAt),
			Model:     parsed.Model,
			Usage:     parsed.Usage.toModelUsage(model.MeasurementDerived),
			Turns:     turns,
		}
		if session.UpdatedAt == nil {
			session.UpdatedAt = fileModTime(ref.Path)
		}
		attachAggregateEvidence(&session.Usage, turns)
		if session.Usage.HasAuthoritativeUsage() {
			session.Evidence = aggregateSessionEvidence(turns)
		}
		sessions = append(sessions, session)
	}
	sortSessions(sessions)
	return sessions, nil
}

func claudeTurns(sessionID string, parsed ParsedSession) []model.Turn {
	if len(parsed.Invocations) == 0 {
		return nil
	}
	turns := make([]model.Turn, 0, len(parsed.Invocations))
	for i, inv := range parsed.Invocations {
		recordID := inv.ID
		if recordID == "" {
			recordID = sessionID + "#" + strconv.Itoa(i+1)
		}
		turn := model.Turn{
			Sequence:  i + 1,
			Timestamp: parseTime(inv.Timestamp),
			Model:     inv.Model,
			Usage:     inv.Snapshot.ToModelUsageWithEvidence(recordID),
			ContextAttribution: model.ContextAttribution{
				Components: inv.Context,
			},
		}
		turn.ID = recordID
		turn.ContextAttribution.Input = model.SeparateInputAccounting(turn.Usage)
		turn.ContextAttribution.Reconciliation = model.ReconcileContext(turn)
		if inv.Conflict {
			conflict := model.Evidence{Kind: "duplicate_conflict", Source: "claude_session"}
			for _, m := range []*model.Measurement{&turn.Usage.Input, &turn.Usage.Cached, &turn.Usage.Output, &turn.Usage.Total} {
				if !m.Available() {
					continue
				}
				m.Evidence = append(m.Evidence, conflict)
			}
		}
		turns = append(turns, turn)
	}
	return turns
}

func attachAggregateEvidence(u *model.Usage, turns []model.Turn) {
	if len(turns) == 0 || !u.HasAuthoritativeUsage() {
		return
	}
	ev := model.Evidence{
		Kind:      model.EvidenceAggregate,
		Source:    "turns",
		Operation: model.AggregateSum,
		Count:     len(turns),
	}
	u.Input.Evidence = append(u.Input.Evidence, ev)
	u.Cached.Evidence = append(u.Cached.Evidence, ev)
	u.Output.Evidence = append(u.Output.Evidence, ev)
	u.Total.Evidence = append(u.Total.Evidence, ev)
}

func aggregateSessionEvidence(turns []model.Turn) []model.Evidence {
	if len(turns) == 0 {
		return nil
	}
	return []model.Evidence{{
		Kind:      model.EvidenceAggregate,
		Source:    "turns",
		Operation: model.AggregateSum,
		Count:     len(turns),
	}}
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

func hasClaudeSessionData(path string) bool {
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
