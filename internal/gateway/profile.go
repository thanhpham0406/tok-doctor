package gateway

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/thanhpham0406/tok-doctor/internal/config"
)

type Profile struct {
	Name        string
	Enabled     bool
	Listen      string
	Protocol    string
	Source      string
	Upstream    string
	ProviderTag string
}

type ProfileSet struct {
	Profiles []Profile
	Errors   []ProfileError
}

type ProfileError struct {
	Name   string
	Reason string
}

func ValidateProfiles(raw map[string]config.GatewayProfile) ProfileSet {
	out := ProfileSet{}
	seenNames := map[string]struct{}{}
	seenListen := map[string]struct{}{}

	for name, p := range raw {
		if _, dup := seenNames[name]; dup {
			out.Errors = append(out.Errors, ProfileError{Name: name, Reason: "duplicate profile name"})
			continue
		}
		seenNames[name] = struct{}{}

		if !p.Enabled {
			continue
		}
		profile := Profile{
			Name:        name,
			Enabled:     p.Enabled,
			Listen:      strings.TrimSpace(p.Listen),
			Protocol:    strings.TrimSpace(p.Protocol),
			Source:      strings.TrimSpace(p.Source),
			Upstream:    strings.TrimSpace(p.Upstream),
			ProviderTag: strings.TrimSpace(p.ProviderTag),
		}
		if err := validateProfile(profile); err != nil {
			out.Errors = append(out.Errors, ProfileError{Name: name, Reason: err.Error()})
			continue
		}
		if _, dup := seenListen[profile.Listen]; dup {
			out.Errors = append(out.Errors, ProfileError{Name: name, Reason: fmt.Sprintf("duplicate listen address %s", profile.Listen)})
			continue
		}
		seenListen[profile.Listen] = struct{}{}
		out.Profiles = append(out.Profiles, profile)
	}
	return out
}

func validateProfile(p Profile) error {
	if p.Listen == "" {
		return fmt.Errorf("listen address is required")
	}
	if !IsLoopbackAddress(p.Listen) {
		return fmt.Errorf("listen address must bind to loopback")
	}
	if p.Protocol == "" {
		return fmt.Errorf("protocol is required")
	}
	if !IsSupportedProtocol(p.Protocol) {
		return fmt.Errorf("unsupported protocol %q", p.Protocol)
	}
	if p.Upstream == "" {
		return fmt.Errorf("upstream is required")
	}
	parsed, err := url.ParseRequestURI(p.Upstream)
	if err != nil {
		return fmt.Errorf("upstream is not a valid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("upstream must use http or https")
	}
	return nil
}

func (s ProfileSet) FilterByName(name string) ProfileSet {
	if name == "" {
		return s
	}
	out := ProfileSet{}
	for _, p := range s.Profiles {
		if p.Name == name {
			out.Profiles = append(out.Profiles, p)
			return out
		}
	}
	out.Errors = append(out.Errors, ProfileError{Name: name, Reason: "profile not found or not enabled"})
	return out
}
