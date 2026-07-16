package apiclient

import (
	"testing"

	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
)

func TestBuildJQL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   JQLParams
		want string
	}{
		{"project only", JQLParams{Project: "ENG"}, `project = "ENG"`},
		{"text only", JQLParams{Text: "login crash"}, `text ~ "login crash"`},
		{"assignee me", JQLParams{Assignee: "me"}, `assignee = currentUser()`},
		{"assignee unassigned", JQLParams{Assignee: "unassigned"}, `assignee is EMPTY`},
		{"reporter me", JQLParams{Reporter: "me"}, `reporter = currentUser()`},
		{"all filters", JQLParams{
			Project: "ENG", Assignee: "alice", Status: "In Progress",
			Type: "Bug", Label: "urgent", Text: "crash",
		}, `project = "ENG" AND assignee = "alice" AND status = "In Progress" AND issuetype = "Bug" AND labels = "urgent" AND text ~ "crash"`},
		{"order by", JQLParams{Project: "ENG", OrderBy: "updated DESC"},
			`project = "ENG" ORDER BY updated DESC`},
		{"escapes quotes", JQLParams{Text: `say "hi"`}, `text ~ "say \"hi\""`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := BuildJQL(tc.in)
			if err != nil {
				t.Fatalf("BuildJQL(%+v) failed: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("BuildJQL(%+v)\n got:  %s\n want: %s", tc.in, got, tc.want)
			}
		})
	}
}

func TestBuildJQLEmpty(t *testing.T) {
	t.Parallel()
	_, err := BuildJQL(JQLParams{})
	if err == nil {
		t.Fatal("BuildJQL(empty) should fail")
	}
	ce := cerrors.AsCLIError(err)
	if ce.Code != "JQL_EMPTY" {
		t.Errorf("code = %s, want JQL_EMPTY", ce.Code)
	}
}

func TestJQLParamsIsEmpty(t *testing.T) {
	t.Parallel()
	if !(JQLParams{}).IsEmpty() {
		t.Error("empty params should report IsEmpty")
	}
	if (JQLParams{Project: "X"}).IsEmpty() {
		t.Error("non-empty params should not report IsEmpty")
	}
}
