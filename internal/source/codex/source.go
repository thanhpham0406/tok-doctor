package codex

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
		finalRecordID := finalSnapshotRecordID(parsed)
		session := model.Session{
			ID:        id,
			Source:    s.Name(),
			Agent:     model.AgentCodex,
			StartedAt: parseTime(parsed.StartedAt),
			UpdatedAt: parseTime(parsed.UpdatedAt),
			Model:     parsed.Model,
			Usage:     parsed.Usage.ToModelUsageWithEvidence(finalRecordID),
			Turns:     reconstructTurns(id, parsed),
		}
		if session.UpdatedAt == nil {
			session.UpdatedAt = fileModTime(ref.Path)
		}
		sessions = append(sessions, session)
	}
	sortSessions(sessions)
	return sessions, nil
}

func finalSnapshotRecordID(parsed ParsedSession) string {
	if len(parsed.Snapshots) == 0 || !parsed.Usage.HasUsage {
		return ""
	}
	for i := len(parsed.Snapshots) - 1; i >= 0; i-- {
		if parsed.Snapshots[i].Snap.Total == parsed.Usage.Total {
			return snapshotRecordID(i + 1)
		}
	}
	return ""
}

func snapshotRecordID(index int) string {
	return "codex_rollout#snap:" + strconv.Itoa(index)
}

func reconstructTurns(sessionID string, parsed ParsedSession) []model.Turn {
	if len(parsed.Snapshots) == 0 {
		return nil
	}
	turns := make([]model.Turn, 0, len(parsed.Snapshots))
	for i, curr := range parsed.Snapshots {
		turn := model.Turn{
			ID:        sessionID + "#" + strconv.Itoa(i+1),
			Sequence:  i + 1,
			Timestamp: parseTime(curr.Timestamp),
			ContextAttribution: model.ContextAttribution{
				Components: curr.Context,
			},
		}
		if parsed.Model != "" {
			turn.Model = parsed.Model
		}
		currRecord := snapshotRecordID(i + 1)
		delta, ok := deltaSnapshot(prevSnapshot(parsed.Snapshots, i), curr)
		if !ok {
			turn.Usage = model.Usage{}
		} else {
			turn.Usage = delta.toModelUsage(model.MeasurementDerived)
			prevRecord := ""
			if i > 0 {
				prevRecord = snapshotRecordID(i)
			}
			attachCumulativeDelta(&turn.Usage, prevRecord, currRecord)
		}
		turn.ContextAttribution.Input = model.CodexInputAccounting(turn.Usage)
		turn.ContextAttribution.Reconciliation = model.ReconcileContext(turn)
		turns = append(turns, turn)
	}
	return turns
}

func prevSnapshot(snaps []snapshot, i int) snapshot {
	if i <= 0 {
		return snapshot{}
	}
	return snaps[i-1]
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
