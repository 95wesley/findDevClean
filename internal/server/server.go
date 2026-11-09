// Package server exposes scan and clean operations over a local HTTP API and
// serves the embedded web UI.
//
// Protections against hostile web pages and other local processes:
//   - Host must be one of AllowedHosts (blocks DNS rebinding).
//   - Origin, when present, must match an allowed host (blocks cross-site requests).
//   - API calls need the per-run token in X-Findclean-Token.
package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"sync"

	"findDevClean/internal/cleaner"
	"findDevClean/internal/rules"
	"findDevClean/internal/scanner"
)

const (
	maxBody  = 1 << 20 // 1 MiB
	maxPaths = 5000
	tokenHdr = "X-Findclean-Token"
)

// Config wires the server to the local machine.
type Config struct {
	Home          string
	Roots         []string
	Rules         []rules.Rule
	QuarantineDir string
	Token         string   // required; empty token rejects every API call
	AllowedHosts  []string // host:port values, e.g. "localhost:7777"
	UI            fs.FS    // optional; nil disables the UI
}

type server struct {
	cfg     Config
	hosts   map[string]bool
	origins map[string]bool

	mu   sync.Mutex
	job  *scanJob // only the latest scan is kept
	tree *treeJob // only the latest disk map is kept
}

// treeJob builds the disk map of the home folder in the background.
type treeJob struct {
	ID     string        `json:"id"`
	State  string        `json:"state"` // running | done
	Done   int           `json:"done"`  // top-level entries measured
	Total  int           `json:"total"` // top-level entries in the home folder
	Tree   *scanner.Node `json:"tree,omitempty"`
	Errors []string      `json:"errors,omitempty"`
}

// mapDepth is how many folder levels the disk map expands.
const mapDepth = 2

// scanJob is one background scan. The client polls it for progress and, when
// State is "done", reads the result from the same response.
type scanJob struct {
	ID       string           `json:"id"`
	State    string           `json:"state"` // running | done | error
	Progress scanner.Progress `json:"progress"`
	Result   *scanner.Result  `json:"result,omitempty"`
	Error    string           `json:"error,omitempty"`
}

// New returns the HTTP handler.
func New(cfg Config) http.Handler {
	s := &server{cfg: cfg, hosts: map[string]bool{}, origins: map[string]bool{}}
	for _, h := range cfg.AllowedHosts {
		h = strings.ToLower(h)
		s.hosts[h] = true
		s.origins["http://"+h] = true
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/rules", s.api(s.handleRules))
	mux.HandleFunc("POST /api/scan", s.api(s.handleScanStart))
	mux.HandleFunc("GET /api/scan/{id}", s.api(s.handleScanStatus))
	mux.HandleFunc("POST /api/tree", s.api(s.handleTreeStart))
	mux.HandleFunc("GET /api/tree/{id}", s.api(s.handleTreeStatus))
	mux.HandleFunc("POST /api/clean", s.api(s.handleClean))
	mux.HandleFunc("GET /api/batches", s.api(s.handleBatches))
	mux.HandleFunc("POST /api/undo", s.api(s.handleUndo))
	if cfg.UI != nil {
		mux.Handle("/", http.FileServerFS(cfg.UI))
	}
	return s.guardHost(mux)
}

func (s *server) guardHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.hosts[strings.ToLower(r.Host)] {
			http.Error(w, "host não permitido", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// api checks Origin and token before running a handler.
func (s *server) api(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" && !s.origins[strings.ToLower(o)] {
			http.Error(w, "origem não permitida", http.StatusForbidden)
			return
		}
		got := r.Header.Get(tokenHdr)
		if s.cfg.Token == "" || subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.Token)) != 1 {
			http.Error(w, "token inválido", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

func (s *server) handleRules(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg.Rules)
}

// handleScanStart starts a scan in the background and returns its id at once,
// so the client can show progress instead of waiting for one long response.
func (s *server) handleScanStart(w http.ResponseWriter, r *http.Request) {
	id, err := newJobID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	job := &scanJob{ID: id, State: "running", Progress: scanner.Progress{Phase: "walk"}}
	s.mu.Lock()
	s.job = job
	s.mu.Unlock()

	go func() {
		res := scanner.Scan(scanner.Options{
			Roots: s.cfg.Roots,
			Rules: s.cfg.Rules,
			Home:  s.cfg.Home,
			OnProgress: func(p scanner.Progress) {
				s.updateJob(id, func(j *scanJob) { j.Progress = p })
			},
		})
		s.updateJob(id, func(j *scanJob) {
			j.State = "done"
			j.Result = &res
			j.Progress = scanner.Progress{Phase: "done", Done: 1, Total: 1}
		})
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"id": id})
}

func (s *server) handleScanStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	job := s.job
	var snapshot scanJob
	if job != nil && job.ID == id {
		snapshot = *job
	}
	s.mu.Unlock()
	if snapshot.ID == "" {
		writeError(w, http.StatusNotFound, "escaneamento não encontrado")
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

// updateJob applies a change to the job with the given id, if it is still the current one.
func (s *server) updateJob(id string, change func(*scanJob)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job != nil && s.job.ID == id {
		change(s.job)
	}
}

// handleTreeStart builds the disk map of the home folder in the background.
func (s *server) handleTreeStart(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Home == "" {
		writeError(w, http.StatusInternalServerError, "pasta home não disponível")
		return
	}
	id, err := newJobID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	job := &treeJob{ID: id, State: "running"}
	s.mu.Lock()
	s.tree = job
	s.mu.Unlock()

	go func() {
		tree, errs := scanner.BuildTree(s.cfg.Home, mapDepth, func(done, total int) {
			s.updateTree(id, func(j *treeJob) { j.Done, j.Total = done, total })
		})
		if len(errs) > 50 {
			errs = errs[:50] // keep the response small; the first errors are enough
		}
		s.updateTree(id, func(j *treeJob) {
			j.State = "done"
			j.Tree = tree
			j.Errors = errs
		})
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"id": id})
}

func (s *server) handleTreeStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	var snapshot treeJob
	if s.tree != nil && s.tree.ID == id {
		snapshot = *s.tree
	}
	s.mu.Unlock()
	if snapshot.ID == "" {
		writeError(w, http.StatusNotFound, "mapa não encontrado")
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *server) updateTree(id string, change func(*treeJob)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tree != nil && s.tree.ID == id {
		change(s.tree)
	}
}

func newJobID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("gerar id do escaneamento: %w", err)
	}
	return hex.EncodeToString(b), nil
}

type cleanRequest struct {
	Paths  []string `json:"paths"`
	DryRun *bool    `json:"dry_run"`
}

func (s *server) handleClean(w http.ResponseWriter, r *http.Request) {
	var req cleanRequest
	if !decode(w, r, &req) {
		return
	}
	if len(req.Paths) == 0 {
		writeError(w, http.StatusBadRequest, "nenhum caminho informado")
		return
	}
	if len(req.Paths) > maxPaths {
		writeError(w, http.StatusBadRequest, "muitos caminhos em uma requisição")
		return
	}
	// Dry run is the default: a client must opt in to actually moving files.
	dry := true
	if req.DryRun != nil {
		dry = *req.DryRun
	}
	rep, err := cleaner.Clean(req.Paths, cleaner.Options{
		Home:          s.cfg.Home,
		Rules:         s.cfg.Rules,
		QuarantineDir: s.cfg.QuarantineDir,
		DryRun:        dry,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (s *server) handleBatches(w http.ResponseWriter, r *http.Request) {
	batches, err := cleaner.ListBatches(s.cfg.QuarantineDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, batches)
}

type undoRequest struct {
	BatchID string `json:"batch_id"`
}

func (s *server) handleUndo(w http.ResponseWriter, r *http.Request) {
	var req undoRequest
	if !decode(w, r, &req) {
		return
	}
	outs, err := cleaner.Undo(s.cfg.QuarantineDir, req.BatchID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"outcomes": outs})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
