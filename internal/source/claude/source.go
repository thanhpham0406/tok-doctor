package claude

import (
	"context"
	"os"
	"path/filepath"
	"sort"

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
		if snap.Total == 0 && snap.Input == 0 && snap.Output == 0 {
			continue
		}
		snaps = append(snaps, snap)
	}
	return SumSnapshots(snaps), nil
}

func (s *Source) capabilities() *source.Capabilities {
	return &source.Capabilities{
		SessionDiscovery: true,
		TokenUsage:       true,
		ToolCalls:        true,
		ToolOutput:       true,
	}
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
