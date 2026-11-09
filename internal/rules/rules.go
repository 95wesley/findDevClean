// Package rules loads the declarative cleanup rules.
package rules

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

//go:embed default.yaml
var defaultYAML []byte

// Risk tells the user how safe it is to delete a match.
type Risk string

const (
	RiskSafe        Risk = "safe"        // regenerated automatically, no cost
	RiskRegenerable Risk = "regenerable" // needs a reinstall or rebuild
	RiskCaution     Risk = "caution"     // may hold data the user cares about
)

// Rule either matches project directories by name (Dir + Markers) or points
// at fixed cache locations under the home directory (Paths, keyed by GOOS).
type Rule struct {
	ID          string              `yaml:"id"`
	Name        string              `yaml:"name"`
	Category    string              `yaml:"category"`
	Risk        Risk                `yaml:"risk"`
	Dir         string              `yaml:"dir"`
	Markers     []string            `yaml:"markers"`
	Paths       map[string][]string `yaml:"paths"`
	Description string              `yaml:"description"`
}

type file struct {
	Rules []Rule `yaml:"rules"`
}

// IsGlobal reports whether the rule refers to fixed cache paths.
func (r Rule) IsGlobal() bool { return len(r.Paths) > 0 }

// GlobalPaths returns the absolute paths a global rule refers to on goos.
func (r Rule) GlobalPaths(home, goos string) []string {
	var out []string
	for _, rel := range r.Paths[goos] {
		out = append(out, filepath.Join(home, filepath.FromSlash(rel)))
	}
	return out
}

// MatchesProjectDir reports whether path is a directory this project rule matches.
func (r Rule) MatchesProjectDir(path string) bool {
	return !r.IsGlobal() && filepath.Base(path) == r.Dir && HasMarkers(filepath.Dir(path), r.Markers)
}

// HasMarkers reports whether dir contains at least one marker file, or whether
// there are no markers at all.
func HasMarkers(dir string, markers []string) bool {
	if len(markers) == 0 {
		return true
	}
	for _, m := range markers {
		if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
			return true
		}
	}
	return false
}

// Default returns the rules embedded in the binary.
func Default() ([]Rule, error) {
	return Parse(defaultYAML)
}

// Parse decodes and validates a rules document.
func Parse(data []byte) ([]Rule, error) {
	var f file
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse rules: %w", err)
	}
	seen := make(map[string]bool, len(f.Rules))
	for i, r := range f.Rules {
		if r.ID == "" {
			return nil, fmt.Errorf("rule %d: id is required", i)
		}
		if seen[r.ID] {
			return nil, fmt.Errorf("rule %q: duplicate id", r.ID)
		}
		seen[r.ID] = true
		if (r.Dir == "") == (len(r.Paths) == 0) {
			return nil, fmt.Errorf("rule %q: set exactly one of dir or paths", r.ID)
		}
		for goos := range r.Paths {
			switch goos {
			case "darwin", "linux", "windows":
			default:
				return nil, fmt.Errorf("rule %q: unknown platform %q", r.ID, goos)
			}
		}
		switch r.Risk {
		case RiskSafe, RiskRegenerable, RiskCaution:
		default:
			return nil, fmt.Errorf("rule %q: invalid risk %q", r.ID, r.Risk)
		}
	}
	return f.Rules, nil
}
