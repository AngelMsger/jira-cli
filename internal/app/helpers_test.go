package app

import (
	"testing"

	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
)

func TestResolveIssueKey(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"bare key", "PROJ-123", "PROJ-123"},
		{"lowercase key", "proj-9", "PROJ-9"},
		{"browse url", "https://acme.atlassian.net/browse/ENG-42", "ENG-42"},
		{"board url", "https://acme.atlassian.net/jira/software/projects/ENG/boards/1?selectedIssue=ENG-8", "ENG-8"},
		{"dc context path", "https://jira.corp.example/jira/browse/OPS-9", "OPS-9"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveIssueKey(tc.in)
			if err != nil {
				t.Fatalf("resolveIssueKey(%q) failed: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("resolveIssueKey(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestResolveIssueKeyRejectsProjectURL(t *testing.T) {
	t.Parallel()
	_, err := resolveIssueKey("https://acme.atlassian.net/browse/ENG")
	if err == nil {
		t.Fatal("a project URL should not resolve to an issue key")
	}
	ce := cerrors.AsCLIError(err)
	if ce.Code != "NO_ISSUE_KEY" {
		t.Errorf("code = %s, want NO_ISSUE_KEY", ce.Code)
	}
}
