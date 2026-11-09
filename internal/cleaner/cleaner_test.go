package cleaner

import (
	"os"
	"path/filepath"
	"testing"

	"findDevClean/internal/rules"
)

func write(t *testing.T, p string, n int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, make([]byte, n), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setup(t *testing.T) (home string, opts Options, nodeModules string) {
	t.Helper()
	home = t.TempDir()
	rs, err := rules.Default()
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(home, "dev", "app")
	write(t, filepath.Join(proj, "package.json"), 2)
	nodeModules = filepath.Join(proj, "node_modules")
	write(t, filepath.Join(nodeModules, "lib", "index.js"), 400)

	opts = Options{
		Home:          home,
		Rules:         rs,
		QuarantineDir: filepath.Join(home, ".findDevClean", "quarantine"),
	}
	return home, opts, nodeModules
}

func TestDryRunTouchesNothing(t *testing.T) {
	_, opts, nm := setup(t)
	opts.DryRun = true

	rep, err := Clean([]string{nm}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if rep.BatchID != "" {
		t.Errorf("dry run must not create a batch, got %q", rep.BatchID)
	}
	if len(rep.Outcomes) != 1 || rep.Outcomes[0].Status != StatusWouldRemove {
		t.Fatalf("outcomes = %+v, want one would_remove", rep.Outcomes)
	}
	if rep.Freed != 400 {
		t.Errorf("freed = %d, want 400", rep.Freed)
	}
	if _, err := os.Stat(nm); err != nil {
		t.Errorf("dry run removed the directory: %v", err)
	}
}

func TestCleanMovesToQuarantineAndUndoRestores(t *testing.T) {
	_, opts, nm := setup(t)

	rep, err := Clean([]string{nm}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if rep.BatchID == "" || rep.Outcomes[0].Status != StatusRemoved {
		t.Fatalf("report = %+v, want removed with batch id", rep)
	}
	if _, err := os.Stat(nm); !os.IsNotExist(err) {
		t.Fatal("original directory should be gone after clean")
	}

	batches, err := ListBatches(opts.QuarantineDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 1 || len(batches[0].Entries) != 1 {
		t.Fatalf("batches = %+v, want one batch with one entry", batches)
	}

	outs, err := Undo(opts.QuarantineDir, rep.BatchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(outs) != 1 || outs[0].Status != StatusRestored {
		t.Fatalf("undo outcomes = %+v", outs)
	}
	if _, err := os.Stat(filepath.Join(nm, "lib", "index.js")); err != nil {
		t.Errorf("file not restored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(opts.QuarantineDir, rep.BatchID)); !os.IsNotExist(err) {
		t.Error("empty batch directory should be removed after full undo")
	}
}

func TestRejectsUnsafePaths(t *testing.T) {
	home, opts, nm := setup(t)

	outside := t.TempDir()
	write(t, filepath.Join(outside, "node_modules", "x"), 10)
	write(t, filepath.Join(outside, "package.json"), 1)

	// A plain folder that matches no rule must never be touched.
	docs := filepath.Join(home, "Documents")
	write(t, filepath.Join(docs, "notes.txt"), 10)

	// A symlink named node_modules pointing at real data.
	linkProj := filepath.Join(home, "dev", "linked")
	write(t, filepath.Join(linkProj, "package.json"), 1)
	link := filepath.Join(linkProj, "node_modules")
	if err := os.Symlink(filepath.Join(docs), link); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	// node_modules with no package.json next to it.
	orphan := filepath.Join(home, "orphan", "node_modules")
	write(t, filepath.Join(orphan, "x"), 10)

	// Something already inside quarantine that would match a rule.
	inQ := filepath.Join(opts.QuarantineDir, "old", "node_modules")
	write(t, filepath.Join(inQ, "x"), 10)
	write(t, filepath.Join(opts.QuarantineDir, "old", "package.json"), 1)

	cases := map[string]string{
		"home itself":   home,
		"outside home":  filepath.Join(outside, "node_modules"),
		"not a rule":    docs,
		"symlink":       link,
		"no marker":     orphan,
		"in quarantine": inQ,
		"relative path": "dev/app/node_modules",
		"missing":       filepath.Join(home, "dev", "gone", "node_modules"),
	}
	for name, p := range cases {
		opts.DryRun = true
		rep, err := Clean([]string{p}, opts)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if rep.Outcomes[0].Status != StatusRejected {
			t.Errorf("%s: status = %s (%s), want rejected", name, rep.Outcomes[0].Status, rep.Outcomes[0].Reason)
		}
	}

	if _, err := os.Stat(filepath.Join(docs, "notes.txt")); err != nil {
		t.Error("non-rule folder was modified")
	}
	if _, err := os.Stat(nm); err != nil {
		t.Error("valid path was modified during a dry run")
	}
}

func TestUndoRejectsTraversalAndOccupiedTargets(t *testing.T) {
	_, opts, nm := setup(t)

	if _, err := Undo(opts.QuarantineDir, "../evil"); err == nil {
		t.Error("expected error for traversal id")
	}

	rep, err := Clean([]string{nm}, opts)
	if err != nil {
		t.Fatal(err)
	}
	// Someone recreates the original location before undo.
	write(t, filepath.Join(nm, "new.txt"), 1)

	outs, err := Undo(opts.QuarantineDir, rep.BatchID)
	if err != nil {
		t.Fatal(err)
	}
	if outs[0].Status != StatusRejected {
		t.Errorf("status = %s, want rejected when target exists", outs[0].Status)
	}
	batches, _ := ListBatches(opts.QuarantineDir)
	if len(batches) != 1 {
		t.Error("batch must be kept when an entry could not be restored")
	}
}
