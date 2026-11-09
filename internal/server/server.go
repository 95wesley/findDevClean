// Package server exposes scan and clean operations over a local HTTP API and
// serves the embedded web UI.
//
// Protections against hostile web pages and other local processes:
//   - Host must be one of AllowedHosts (blocks DNS rebinding).
//   - Origin, when present, must match an allowed host (blocks cross-site requests).
//   - API calls need the per-run token in X-Findclean-Token.
package server

import (
	"crypto/subtle"
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"

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
	mux.HandleFunc("POST /api/scan", s.api(s.handleScan))
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

func (s *server) handleScan(w http.ResponseWriter, r *http.Request) {
	res := scanner.Scan(scanner.Options{
		Roots: s.cfg.Roots,
		Rules: s.cfg.Rules,
		Home:  s.cfg.Home,
	})
	writeJSON(w, http.StatusOK, res)
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
