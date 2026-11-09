package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"findDevClean/internal/rules"
)

const token = "test-token-123"

func newHandler(t *testing.T) (http.Handler, string) {
	t.Helper()
	home := t.TempDir()
	rs, err := rules.Default()
	if err != nil {
		t.Fatal(err)
	}
	nm := filepath.Join(home, "app", "node_modules")
	if err := os.MkdirAll(nm, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(home, "app", "package.json"), []byte("{}"), 0o644)
	os.WriteFile(filepath.Join(nm, "x.js"), make([]byte, 256), 0o644)

	h := New(Config{
		Home:          home,
		Roots:         []string{home},
		Rules:         rs,
		QuarantineDir: filepath.Join(home, ".findDevClean", "quarantine"),
		Token:         token,
		AllowedHosts:  []string{"localhost:7777", "127.0.0.1:7777"},
	})
	return h, nm
}

func do(h http.Handler, method, path, host, origin, tok string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Host = host
	req.Header.Set("Content-Type", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if tok != "" {
		req.Header.Set(tokenHdr, tok)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRejectsForeignHost(t *testing.T) {
	h, _ := newHandler(t)
	// DNS rebinding: attacker domain resolves to 127.0.0.1.
	rec := do(h, "POST", "/api/scan", "evil.example:7777", "", token, nil)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestRejectsForeignOrigin(t *testing.T) {
	h, _ := newHandler(t)
	rec := do(h, "POST", "/api/scan", "localhost:7777", "https://evil.example", token, nil)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestRequiresToken(t *testing.T) {
	h, _ := newHandler(t)
	for _, tok := range []string{"", "wrong"} {
		rec := do(h, "POST", "/api/scan", "localhost:7777", "", tok, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("token %q: status = %d, want 401", tok, rec.Code)
		}
	}
}

func TestEmptyConfiguredTokenRejectsEverything(t *testing.T) {
	h := New(Config{Home: t.TempDir(), Token: "", AllowedHosts: []string{"localhost:7777"}})
	rec := do(h, "GET", "/api/rules", "localhost:7777", "", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestScanReturnsFindings(t *testing.T) {
	h, _ := newHandler(t)
	rec := do(h, "POST", "/api/scan", "localhost:7777", "http://localhost:7777", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body)
	}
	var res struct {
		Findings []struct {
			RuleID string `json:"rule_id"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) != 1 || res.Findings[0].RuleID != "node-modules" {
		t.Errorf("findings = %+v, want one node-modules", res.Findings)
	}
}

func TestCleanDefaultsToDryRun(t *testing.T) {
	h, nm := newHandler(t)
	rec := do(h, "POST", "/api/clean", "localhost:7777", "", token, map[string]any{
		"paths": []string{nm},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"dry_run":true`) {
		t.Errorf("response not dry run: %s", rec.Body)
	}
	if _, err := os.Stat(nm); err != nil {
		t.Error("request without dry_run=false must not move files")
	}
}

func TestCleanApplyThenUndo(t *testing.T) {
	h, nm := newHandler(t)
	dryRun := false
	rec := do(h, "POST", "/api/clean", "localhost:7777", "", token, map[string]any{
		"paths": []string{nm}, "dry_run": dryRun,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("clean status = %d body = %s", rec.Code, rec.Body)
	}
	var rep struct {
		BatchID string `json:"batch_id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &rep)
	if rep.BatchID == "" {
		t.Fatalf("no batch id in %s", rec.Body)
	}
	if _, err := os.Stat(nm); !os.IsNotExist(err) {
		t.Fatal("directory should have been moved")
	}

	rec = do(h, "POST", "/api/undo", "localhost:7777", "", token, map[string]any{"batch_id": rep.BatchID})
	if rec.Code != http.StatusOK {
		t.Fatalf("undo status = %d body = %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(nm); err != nil {
		t.Error("undo did not restore the directory")
	}
}

func TestRejectsUnknownFieldsAndEmptyPaths(t *testing.T) {
	h, _ := newHandler(t)
	rec := do(h, "POST", "/api/clean", "localhost:7777", "", token, map[string]any{"paths": []string{}})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty paths: status = %d, want 400", rec.Code)
	}

	req := httptest.NewRequest("POST", "/api/clean", strings.NewReader(`{"paths":["/x"],"force":true}`))
	req.Host = "localhost:7777"
	req.Header.Set(tokenHdr, token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown field: status = %d, want 400", rec.Code)
	}
}
