// Package cleaner moves matched directories into a quarantine so every
// removal can be undone. Nothing is deleted outright.
package cleaner

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"findDevClean/internal/rules"
	"findDevClean/internal/scanner"
)

const manifestName = "manifest.json"

// Options configures a clean run.
type Options struct {
	Home          string       // boundary: only paths under Home are accepted
	Rules         []rules.Rule // a path must match one of these to be accepted
	QuarantineDir string       // where batches are stored
	DryRun        bool         // report what would move, touch nothing
}

// Status is the result of one path.
type Status string

const (
	StatusWouldRemove Status = "would_remove"
	StatusRemoved     Status = "removed"
	StatusRestored    Status = "restored"
	StatusRejected    Status = "rejected"
	StatusFailed      Status = "failed"
)

// Outcome describes what happened to one requested path.
type Outcome struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Status Status `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// Report summarises a clean run.
type Report struct {
	BatchID  string    `json:"batch_id,omitempty"`
	DryRun   bool      `json:"dry_run"`
	Freed    int64     `json:"freed"`
	Outcomes []Outcome `json:"outcomes"`
}

// Entry records where a removed directory came from and where it sits now.
type Entry struct {
	Original    string `json:"original"`
	Quarantined string `json:"quarantined"`
	Size        int64  `json:"size"`
}

// Batch is one clean run that moved at least one directory.
type Batch struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Entries   []Entry   `json:"entries"`
}

// Clean validates each path and, unless DryRun is set, moves it into a new
// quarantine batch. Invalid paths are rejected individually; they never abort
// the run.
func Clean(paths []string, opts Options) (Report, error) {
	rep := Report{DryRun: opts.DryRun, Outcomes: []Outcome{}}
	if opts.QuarantineDir == "" {
		return rep, errors.New("quarantine directory is not set")
	}

	var batch Batch
	var batchDir string
	if !opts.DryRun {
		batch.ID = time.Now().Format("20060102-150405.000000000")
		batch.CreatedAt = time.Now().UTC()
		batchDir = filepath.Join(opts.QuarantineDir, batch.ID)
		if err := os.MkdirAll(batchDir, 0o755); err != nil {
			return rep, fmt.Errorf("create batch: %w", err)
		}
		rep.BatchID = batch.ID
	}

	for i, raw := range paths {
		p := filepath.Clean(raw)
		size, err := validate(p, opts)
		if err != nil {
			rep.Outcomes = append(rep.Outcomes, Outcome{Path: raw, Status: StatusRejected, Reason: err.Error()})
			continue
		}

		if opts.DryRun {
			rep.Outcomes = append(rep.Outcomes, Outcome{Path: p, Size: size, Status: StatusWouldRemove})
			rep.Freed += size
			continue
		}

		dest := filepath.Join(batchDir, fmt.Sprintf("%03d-%s", i, filepath.Base(p)))
		if err := os.Rename(p, dest); err != nil {
			rep.Outcomes = append(rep.Outcomes, Outcome{Path: p, Size: size, Status: StatusFailed, Reason: err.Error()})
			continue
		}
		batch.Entries = append(batch.Entries, Entry{Original: p, Quarantined: dest, Size: size})
		rep.Freed += size
		rep.Outcomes = append(rep.Outcomes, Outcome{Path: p, Size: size, Status: StatusRemoved})

		// Write the manifest after every move so an interrupted run can still be undone.
		if err := writeManifest(batchDir, batch); err != nil {
			return rep, err
		}
	}

	if !opts.DryRun && len(batch.Entries) == 0 {
		_ = os.RemoveAll(batchDir)
		rep.BatchID = ""
	}
	return rep, nil
}

// validate applies every safety check and returns the size of the directory.
func validate(p string, opts Options) (int64, error) {
	if !filepath.IsAbs(p) {
		return 0, errors.New("caminho precisa ser absoluto")
	}
	home := filepath.Clean(opts.Home)
	if !within(home, p) || p == home {
		return 0, errors.New("fora da pasta home")
	}
	if opts.QuarantineDir != "" && within(filepath.Clean(opts.QuarantineDir), p) {
		return 0, errors.New("já está na quarentena")
	}
	info, err := os.Lstat(p)
	if err != nil {
		return 0, errors.New("não existe")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return 0, errors.New("é um link simbólico")
	}
	if !info.IsDir() {
		return 0, errors.New("não é um diretório")
	}
	if !matchesRule(p, opts) {
		return 0, errors.New("não corresponde a nenhuma regra de limpeza")
	}
	size, _ := scanner.SizeOf(p)
	return size, nil
}

// within reports whether p is strictly inside dir.
func within(dir, p string) bool {
	rel, err := filepath.Rel(dir, p)
	if err != nil || rel == "." || rel == ".." {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func matchesRule(p string, opts Options) bool {
	for _, r := range opts.Rules {
		if r.IsGlobal() {
			for _, g := range r.GlobalPaths(opts.Home, runtime.GOOS) {
				if g == p {
					return true
				}
			}
		} else if r.MatchesProjectDir(p) {
			return true
		}
	}
	return false
}

// Undo moves every entry of a batch back to its original location. Entries
// whose original location is occupied again are left in quarantine.
func Undo(quarantineDir, batchID string) ([]Outcome, error) {
	if batchID == "" || batchID == "." || batchID == ".." || strings.ContainsAny(batchID, `/\`) {
		return nil, errors.New("id de lote inválido")
	}
	dir := filepath.Join(quarantineDir, batchID)
	batch, err := readManifest(dir)
	if err != nil {
		return nil, err
	}

	outs := []Outcome{}
	var remaining []Entry
	for _, e := range batch.Entries {
		if _, err := os.Lstat(e.Original); err == nil {
			outs = append(outs, Outcome{Path: e.Original, Size: e.Size, Status: StatusRejected, Reason: "destino já existe"})
			remaining = append(remaining, e)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(e.Original), 0o755); err != nil {
			outs = append(outs, Outcome{Path: e.Original, Size: e.Size, Status: StatusFailed, Reason: err.Error()})
			remaining = append(remaining, e)
			continue
		}
		if err := os.Rename(e.Quarantined, e.Original); err != nil {
			outs = append(outs, Outcome{Path: e.Original, Size: e.Size, Status: StatusFailed, Reason: err.Error()})
			remaining = append(remaining, e)
			continue
		}
		outs = append(outs, Outcome{Path: e.Original, Size: e.Size, Status: StatusRestored})
	}

	if len(remaining) == 0 {
		return outs, os.RemoveAll(dir)
	}
	batch.Entries = remaining
	return outs, writeManifest(dir, batch)
}

// ListBatches returns quarantine batches, newest first.
func ListBatches(quarantineDir string) ([]Batch, error) {
	out := []Batch{}
	entries, err := os.ReadDir(quarantineDir)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		b, err := readManifest(filepath.Join(quarantineDir, e.Name()))
		if err != nil {
			continue
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func writeManifest(dir string, b Batch) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, manifestName), data, 0o644)
}

func readManifest(dir string) (Batch, error) {
	var b Batch
	data, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		return b, fmt.Errorf("lote não encontrado: %w", err)
	}
	if err := json.Unmarshal(data, &b); err != nil {
		return b, fmt.Errorf("manifesto inválido: %w", err)
	}
	return b, nil
}
