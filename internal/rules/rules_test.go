package rules

import "testing"

func TestDefaultRulesLoad(t *testing.T) {
	r, err := Default()
	if err != nil {
		t.Fatalf("Default() error: %v", err)
	}
	if len(r) < 10 {
		t.Fatalf("default rules = %d, want at least 10 (3 project + global caches)", len(r))
	}
	var global int
	for _, rule := range r {
		if rule.IsGlobal() {
			global++
		}
	}
	if global == 0 {
		t.Fatal("expected global cache rules")
	}
}

func TestParseRejectsInvalid(t *testing.T) {
	cases := map[string]string{
		"missing id":    "rules:\n  - dir: x\n    risk: safe\n",
		"missing dir":   "rules:\n  - id: a\n    risk: safe\n",
		"dir and paths": "rules:\n  - id: a\n    dir: x\n    paths: {linux: [.x]}\n    risk: safe\n",
		"unknown os":    "rules:\n  - id: a\n    paths: {plan9: [.x]}\n    risk: safe\n",
		"bad risk":      "rules:\n  - id: a\n    dir: x\n    risk: nuke\n",
		"duplicate id":  "rules:\n  - id: a\n    dir: x\n    risk: safe\n  - id: a\n    dir: y\n    risk: safe\n",
	}
	for name, doc := range cases {
		if _, err := Parse([]byte(doc)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
