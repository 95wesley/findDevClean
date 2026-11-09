// Package scanner walks directory trees and reports artifacts matched by rules.
package scanner

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"findDevClean/internal/rules"
)

// DefaultMaxDepth limits how deep the walk descends below each root.
const DefaultMaxDepth = 8

// DefaultSkipDirs are never entered. Library and .Trash are large macOS
// trees that do not hold project artifacts. .findDevClean holds our own
// quarantine, which must never be scanned back.
var DefaultSkipDirs = map[string]bool{
	".git":          true,
	".Trash":        true,
	"Library":       true,
	".findDevClean": true,
}

// Options configures a scan.
type Options struct {
	Roots    []string
	Rules    []rules.Rule
	Home     string          // enables global cache rules when set
	MaxDepth int             // <= 0 uses DefaultMaxDepth
	SkipDirs map[string]bool // nil uses DefaultSkipDirs
	// OnProgress, when set, receives updates while the scan runs.
	OnProgress func(Progress)
}

// Progress describes where a scan is. Phases run in order: "walk" (project
// folders), "caches" (fixed cache paths), then "done".
//
// In the walk phase, Total counts the top-level entries of every root, so the
// percentage is an estimate: nested folders are not known in advance.
type Progress struct {
	Phase   string `json:"phase"`
	Done    int    `json:"done"`
	Total   int    `json:"total"`
	Current string `json:"current,omitempty"`
}

// Finding is one matched directory or cache location.
type Finding struct {
	RuleID      string     `json:"rule_id"`
	Name        string     `json:"name"`
	Category    string     `json:"category"`
	Risk        rules.Risk `json:"risk"`
	Path        string     `json:"path"`
	ProjectDir  string     `json:"project_dir,omitempty"`
	Size        int64      `json:"size"`
	Description string     `json:"description"`
}

// Result is the outcome of a scan. Errors are non-fatal (permission denied,
// vanished files) and do not stop the walk.
type Result struct {
	Findings []Finding `json:"findings"`
	Total    int64     `json:"total"`
	Errors   []string  `json:"errors"`
}

// Scan walks every root for project rules, then checks global cache paths
// under Home. Findings are sorted by size, largest first.
func Scan(opts Options) Result {
	maxDepth := opts.MaxDepth
	if maxDepth <= 0 {
		maxDepth = DefaultMaxDepth
	}
	skip := opts.SkipDirs
	if skip == nil {
		skip = DefaultSkipDirs
	}

	byDir := make(map[string][]rules.Rule)
	for _, r := range opts.Rules {
		if !r.IsGlobal() {
			byDir[r.Dir] = append(byDir[r.Dir], r)
		}
	}

	res := Result{Findings: []Finding{}, Errors: []string{}}

	emit := func(p Progress) {
		if opts.OnProgress != nil {
			opts.OnProgress(p)
		}
	}

	// Estimate the walk: top-level entries of every existing root.
	walkTotal := 0
	for _, root := range opts.Roots {
		if entries, err := os.ReadDir(filepath.Clean(root)); err == nil {
			walkTotal += len(entries)
		}
	}
	walkDone := 0
	emit(Progress{Phase: "walk", Done: 0, Total: walkTotal})

	for _, root := range opts.Roots {
		root = filepath.Clean(root)
		if _, err := os.Stat(root); err != nil {
			res.Errors = append(res.Errors, err.Error())
			continue
		}

		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				res.Errors = append(res.Errors, err.Error())
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if path != root && filepath.Dir(path) == root {
				walkDone++
				emit(Progress{Phase: "walk", Done: walkDone, Total: walkTotal, Current: path})
			}
			// WalkDir does not follow symlinks, so symlinked dirs are never
			// entered and never counted.
			if !d.IsDir() {
				return nil
			}
			if path != root && skip[d.Name()] {
				return fs.SkipDir
			}
			if depth(root, path) > maxDepth {
				return fs.SkipDir
			}
			if path == root {
				return nil
			}

			rule, ok := match(byDir[d.Name()], path)
			if !ok {
				return nil
			}

			size, errs := SizeOf(path)
			res.Errors = append(res.Errors, errs...)
			res.Findings = append(res.Findings, Finding{
				RuleID:      rule.ID,
				Name:        rule.Name,
				Category:    rule.Category,
				Risk:        rule.Risk,
				Path:        path,
				ProjectDir:  filepath.Dir(path),
				Size:        size,
				Description: rule.Description,
			})
			res.Total += size
			// Do not descend into a match: nested node_modules inside
			// node_modules would otherwise be counted twice.
			return fs.SkipDir
		})
	}

	if opts.Home != "" {
		addGlobal(&res, opts.Rules, opts.Home, emit)
	}
	emit(Progress{Phase: "done", Done: 1, Total: 1})

	sort.SliceStable(res.Findings, func(i, j int) bool {
		return res.Findings[i].Size > res.Findings[j].Size
	})
	return res
}

// addGlobal checks each fixed cache path. Missing caches are normal and are
// silently skipped.
func addGlobal(res *Result, all []rules.Rule, home string, emit func(Progress)) {
	// Collect the paths first so progress can report "k of n".
	var paths []struct {
		rule rules.Rule
		path string
	}
	for _, r := range all {
		if !r.IsGlobal() {
			continue
		}
		for _, p := range r.GlobalPaths(home, runtime.GOOS) {
			paths = append(paths, struct {
				rule rules.Rule
				path string
			}{r, p})
		}
	}

	seen := make(map[string]bool)
	for i, item := range paths {
		r, p := item.rule, item.path
		emit(Progress{Phase: "caches", Done: i, Total: len(paths), Current: p})
		{
			if seen[p] {
				continue
			}
			seen[p] = true
			info, err := os.Lstat(p)
			if err != nil || info.Mode()&os.ModeSymlink != 0 {
				continue
			}
			var size int64
			switch {
			case info.IsDir():
				s, errs := SizeOf(p)
				size = s
				res.Errors = append(res.Errors, errs...)
			case info.Mode().IsRegular():
				size = diskSize(info)
			default:
				continue
			}
			res.Findings = append(res.Findings, Finding{
				RuleID:      r.ID,
				Name:        r.Name,
				Category:    r.Category,
				Risk:        r.Risk,
				Path:        p,
				Size:        size,
				Description: r.Description,
			})
			res.Total += size
		}
	}
}

// match returns the first project rule that matches path.
func match(candidates []rules.Rule, path string) (rules.Rule, bool) {
	for _, r := range candidates {
		if r.MatchesProjectDir(path) {
			return r, true
		}
	}
	return rules.Rule{}, false
}

// depth counts path segments between root and path. root itself is 0.
func depth(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return 0
	}
	return strings.Count(rel, string(filepath.Separator)) + 1
}

// SizeOf sums regular file sizes under dir without following symlinks.
func SizeOf(dir string) (int64, []string) {
	var total int64
	var errs []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			errs = append(errs, err.Error())
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			errs = append(errs, err.Error())
			return nil
		}
		if info.Mode().IsRegular() {
			total += diskSize(info)
		}
		return nil
	})
	return total, errs
}
