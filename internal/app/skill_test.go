package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentIDs(t *testing.T) {
	t.Parallel()
	want := []string{
		"claude-code", "codex", "cursor", "agents", "gemini", "github-copilot",
		"opencode", "continue", "windsurf", "grok", "pi", "kilo", "roo",
	}
	ids := agentIDs()
	if len(ids) != len(want) {
		t.Fatalf("agentIDs() = %v (%d), want %d entries", ids, len(ids), len(want))
	}
	got := map[string]bool{}
	for _, id := range ids {
		got[id] = true
	}
	for _, id := range want {
		if !got[id] {
			t.Errorf("missing agent id %q", id)
		}
	}
}

func TestAgentDests(t *testing.T) {
	t.Parallel()
	cases := []struct {
		id              string
		wantHomeSuffix  string
		wantProjectPath string
	}{
		{"claude-code", filepath.Join(".claude", "skills", "jira"), filepath.Join(".claude", "skills", "jira")},
		{"codex", filepath.Join(".codex", "skills", "jira"), filepath.Join(".agents", "skills", "jira")},
		{"cursor", filepath.Join(".cursor", "skills", "jira"), filepath.Join(".cursor", "skills", "jira")},
		{"agents", filepath.Join(".agents", "skills", "jira"), filepath.Join(".agents", "skills", "jira")},
		{"gemini", filepath.Join(".gemini", "skills", "jira"), filepath.Join(".gemini", "skills", "jira")},
		{"github-copilot", filepath.Join(".copilot", "skills", "jira"), filepath.Join(".agents", "skills", "jira")},
		{"opencode", filepath.Join(".config", "opencode", "skills", "jira"), filepath.Join(".opencode", "skills", "jira")},
		{"continue", filepath.Join(".continue", "skills", "jira"), filepath.Join(".continue", "skills", "jira")},
		{"windsurf", filepath.Join(".codeium", "windsurf", "skills", "jira"), filepath.Join(".windsurf", "skills", "jira")},
		{"grok", filepath.Join(".grok", "skills", "jira"), filepath.Join(".grok", "skills", "jira")},
		{"pi", filepath.Join(".pi", "agent", "skills", "jira"), filepath.Join(".pi", "skills", "jira")},
		{"kilo", filepath.Join(".kilocode", "skills", "jira"), filepath.Join(".kilocode", "skills", "jira")},
		{"roo", filepath.Join(".roo", "skills", "jira"), filepath.Join(".roo", "skills", "jira")},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			spec, ok := agentByID(tc.id)
			if !ok {
				t.Fatalf("agentSpec %q missing", tc.id)
			}
			projectPath, err := agentDest(spec, true)
			if err != nil {
				t.Fatal(err)
			}
			if projectPath != tc.wantProjectPath {
				t.Fatalf("project dest = %q, want %q", projectPath, tc.wantProjectPath)
			}
			homePath, err := agentDest(spec, false)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(homePath, tc.wantHomeSuffix) {
				t.Fatalf("home dest %q does not end with %q", homePath, tc.wantHomeSuffix)
			}
		})
	}
}
