package errors

import "testing"

// The model is tested in agent-cli-kit; this checks that jira-cli's own
// guidance is what an error built through this package carries.
func TestErrorsCarryJiraGuidance(t *testing.T) {
	e := New(CategoryConfig, "BAD_BASE_URL", "service URL must be an HTTP(S) URL")
	want := []string{"jira-cli config init", "jira-cli config show --explain"}
	if len(e.NextSteps) != len(want) || e.NextSteps[0] != want[0] || e.NextSteps[1] != want[1] {
		t.Fatalf("NextSteps = %v, want jira-cli's config guidance %v", e.NextSteps, want)
	}
	if ExitCode(e) != ExitConfig {
		t.Fatalf("ExitCode = %d, want %d", ExitCode(e), ExitConfig)
	}
}
