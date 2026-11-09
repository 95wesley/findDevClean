package scanner

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"findDevClean/internal/rules"
)

// writeFile creates path under root with n bytes of content and returns the
// space it occupies on disk, which is what the scanner reports.
func writeFile(t *testing.T, root, rel string, n int) int64 {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, make([]byte, n), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return diskSize(info)
}

func defaultRules(t *testing.T) []rules.Rule {
	t.Helper()
	r, err := rules.Default()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func findingsByID(res Result) map[string][]Finding {
	m := map[string][]Finding{}
	for _, f := range res.Findings {
		m[f.RuleID] = append(m[f.RuleID], f)
	}
	return m
}

func TestScanMatchesWithMarkers(t *testing.T) {
	root := t.TempDir()

	// Real JS project: node_modules has package.json next to it.
	writeFile(t, root, "js-app/package.json", 10)
	nmSize := writeFile(t, root, "js-app/node_modules/lib/index.js", 100)
	// Nested node_modules inside node_modules must not be counted twice.
	nmSize += writeFile(t, root, "js-app/node_modules/lib/node_modules/x/y.js", 50)

	// node_modules without package.json is not a match.
	writeFile(t, root, "loose/node_modules/a.js", 999)

	// Rust project with Cargo.toml.
	writeFile(t, root, "rs-app/Cargo.toml", 10)
	targetSize := writeFile(t, root, "rs-app/target/debug/bin", 200)

	// target without Cargo.toml is not a match.
	writeFile(t, root, "not-rust/target/out", 300)

	// pycache needs no marker.
	pycacheSize := writeFile(t, root, "py/src/__pycache__/m.pyc", 40)

	res := Scan(Options{Roots: []string{root}, Rules: defaultRules(t)})
	byID := findingsByID(res)

	if n := len(byID["node-modules"]); n != 1 {
		t.Fatalf("node-modules matches = %d, want 1", n)
	}
	if got := byID["node-modules"][0].Size; got != nmSize {
		t.Errorf("node_modules size = %d, want %d (nested must not double count)", got, nmSize)
	}
	if n := len(byID["rust-target"]); n != 1 {
		t.Fatalf("rust-target matches = %d, want 1", n)
	}
	if got := byID["rust-target"][0].Size; got != targetSize {
		t.Errorf("target size = %d, want %d", got, targetSize)
	}
	if n := len(byID["python-pycache"]); n != 1 {
		t.Fatalf("python-pycache matches = %d, want 1", n)
	}
	if want := nmSize + targetSize + pycacheSize; res.Total != want {
		t.Errorf("total = %d, want %d", res.Total, want)
	}
}

func TestScanSortsBySizeDesc(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a/package.json", 1)
	writeFile(t, root, "a/node_modules/x", 10)
	writeFile(t, root, "b/package.json", 1)
	writeFile(t, root, "b/node_modules/x", 500)

	res := Scan(Options{Roots: []string{root}, Rules: defaultRules(t)})
	if len(res.Findings) != 2 {
		t.Fatalf("findings = %d, want 2", len(res.Findings))
	}
	if res.Findings[0].Size < res.Findings[1].Size {
		t.Errorf("not sorted desc: %d then %d", res.Findings[0].Size, res.Findings[1].Size)
	}
}

func TestScanRespectsSkipAndDepth(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "Library/proj/package.json", 1)
	writeFile(t, root, "Library/proj/node_modules/x", 10)
	writeFile(t, root, "shallow/package.json", 1)
	writeFile(t, root, "shallow/node_modules/x", 10)

	// Deeper than MaxDepth=2: the artifact at depth 4 must be ignored.
	writeFile(t, root, "a/b/c/d/package.json", 1)
	writeFile(t, root, "a/b/c/d/node_modules/x", 10)

	res := Scan(Options{
		Roots:    []string{root},
		Rules:    defaultRules(t),
		MaxDepth: 2,
		SkipDirs: map[string]bool{"Library": true},
	})
	if len(res.Findings) != 1 {
		t.Fatalf("findings = %d, want 1 (only shallow)", len(res.Findings))
	}
	if !strings.Contains(res.Findings[0].Path, "shallow") {
		t.Errorf("unexpected match: %s", res.Findings[0].Path)
	}
}

func TestScanDoesNotFollowSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFile(t, outside, "big/data", 5000)

	writeFile(t, root, "proj/package.json", 1)
	if err := os.Symlink(filepath.Join(outside, "big"), filepath.Join(root, "proj", "node_modules")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	res := Scan(Options{Roots: []string{root}, Rules: defaultRules(t)})
	if len(res.Findings) != 0 {
		t.Errorf("symlinked node_modules must not be matched, got %d findings", len(res.Findings))
	}
}

func TestScanGlobalCachesUnderHome(t *testing.T) {
	home := t.TempDir()
	rs := defaultRules(t)

	// Create the npm cache at the path the rule resolves to on this OS.
	var npm rules.Rule
	for _, r := range rs {
		if r.ID == "npm-cache" {
			npm = r
		}
	}
	cache := npm.GlobalPaths(home, runtime.GOOS)
	if len(cache) == 0 {
		t.Skipf("npm-cache has no path on %s", runtime.GOOS)
	}
	want := writeFile(t, filepath.Dir(cache[0]), filepath.Base(cache[0])+"/content-v2/x", 700)

	res := Scan(Options{Roots: []string{}, Rules: rs, Home: home})
	byID := findingsByID(res)
	if n := len(byID["npm-cache"]); n != 1 {
		t.Fatalf("npm-cache findings = %d, want 1", n)
	}
	if got := byID["npm-cache"][0].Size; got != want {
		t.Errorf("npm-cache size = %d, want %d", got, want)
	}
	// Missing caches (pip, go-build, ...) must not produce findings.
	if len(byID["pip-cache"]) != 0 {
		t.Error("missing pip cache should not be reported")
	}
}

func TestScanWithoutHomeSkipsGlobal(t *testing.T) {
	res := Scan(Options{Roots: []string{}, Rules: defaultRules(t)})
	if len(res.Findings) != 0 {
		t.Errorf("findings = %d, want 0 when Home is unset", len(res.Findings))
	}
}

func TestScanMissingRootIsNonFatal(t *testing.T) {
	res := Scan(Options{Roots: []string{filepath.Join(t.TempDir(), "nope")}, Rules: defaultRules(t)})
	if len(res.Errors) == 0 {
		t.Error("expected an error for a missing root")
	}
	if len(res.Findings) != 0 {
		t.Errorf("findings = %d, want 0", len(res.Findings))
	}
}
