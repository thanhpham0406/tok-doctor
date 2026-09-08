package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Config struct {
	Sources map[string]Source `json:"sources,omitempty"`
	Pricing Pricing           `json:"pricing,omitempty"`
}

type Source struct {
	Path     string `json:"path,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
}

type Pricing struct {
	OverridePath string `json:"override_path,omitempty"`
}

type Store struct {
	path string
}

func NewStore() (Store, error) {
	if path := os.Getenv("TOKDOCTOR_CONFIG"); path != "" {
		return Store{path: path}, nil
	}

	dir, err := os.UserConfigDir()
	if err != nil {
		return Store{}, fmt.Errorf("find user config dir: %w", err)
	}
	return Store{path: filepath.Join(dir, "tokdoctor", "config.toml")}, nil
}

func NewStoreAt(path string) Store {
	return Store{path: path}
}

func (s Store) Path() string {
	return s.path
}

func (s Store) Load() (Config, error) {
	cfg := Config{Sources: map[string]Source{}}
	if s.path == "" {
		return cfg, nil
	}

	file, err := os.Open(s.path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("open config %s: %w", s.path, err)
	}
	defer file.Close()

	current := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
			if section == "pricing" {
				current = "pricing"
			} else {
				current = sectionSource(section)
			}
			continue
		}
		if current == "" {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return Config{}, fmt.Errorf("parse config %s: invalid assignment %q", s.path, line)
		}
		key = strings.TrimSpace(key)
		parsed, err := parseString(strings.TrimSpace(value))
		if err != nil {
			return Config{}, fmt.Errorf("parse config %s: %w", s.path, err)
		}

		if current == "pricing" {
			if key == "override_path" {
				cfg.Pricing.OverridePath = parsed
			}
			continue
		}

		src := cfg.Sources[current]
		switch key {
		case "path":
			src.Path = parsed
		case "endpoint":
			src.Endpoint = parsed
		default:
			continue
		}
		cfg.Sources[current] = src
	}
	if err := scanner.Err(); err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", s.path, err)
	}

	return cfg, nil
}

func (s Store) Save(cfg Config) error {
	if s.path == "" {
		return fmt.Errorf("config path is unavailable")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	var b strings.Builder
	names := make([]string, 0, len(cfg.Sources))
	for name, src := range cfg.Sources {
		if src.Path == "" && src.Endpoint == "" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	if cfg.Pricing.OverridePath != "" {
		b.WriteString("[pricing]\n")
		b.WriteString("override_path = ")
		b.WriteString(quoteString(cfg.Pricing.OverridePath))
		b.WriteByte('\n')
		if len(names) > 0 {
			b.WriteByte('\n')
		}
	}

	for i, name := range names {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("[sources.")
		b.WriteString(name)
		b.WriteString("]\n")

		src := cfg.Sources[name]
		if src.Path != "" {
			b.WriteString("path = ")
			b.WriteString(quoteString(src.Path))
			b.WriteByte('\n')
		}
		if src.Endpoint != "" {
			b.WriteString("endpoint = ")
			b.WriteString(quoteString(src.Endpoint))
			b.WriteByte('\n')
		}
	}

	if err := os.WriteFile(s.path, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("write config %s: %w", s.path, err)
	}
	return nil
}

func (s Store) SetPricingOverride(path string) error {
	cfg, err := s.Load()
	if err != nil {
		return err
	}
	cfg.Pricing.OverridePath = path
	return s.Save(cfg)
}

func (s Store) SetSource(name string, src Source) error {
	cfg, err := s.Load()
	if err != nil {
		return err
	}
	cfg.Sources[name] = src
	return s.Save(cfg)
}

func (s Store) ResetSource(name string) error {
	cfg, err := s.Load()
	if err != nil {
		return err
	}
	delete(cfg.Sources, name)
	return s.Save(cfg)
}

func sectionSource(section string) string {
	if !strings.HasPrefix(section, "sources.") {
		return ""
	}
	return strings.TrimPrefix(section, "sources.")
}

func parseString(raw string) (string, error) {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return "", fmt.Errorf("expected quoted TOML string")
	}
	unquoted := raw[1 : len(raw)-1]
	unquoted = strings.ReplaceAll(unquoted, `\\`, `\`)
	unquoted = strings.ReplaceAll(unquoted, `\"`, `"`)
	return unquoted, nil
}

func quoteString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}
