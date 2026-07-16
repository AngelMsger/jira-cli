package urlref

import "testing"

func TestParse(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want Ref
	}{
		{"empty", "", Ref{}},
		{"bare key", "PROJ-123", Ref{IssueKey: "PROJ-123", ProjectKey: "PROJ"}},
		{"bare key lowercased", "proj-7", Ref{IssueKey: "PROJ-7", ProjectKey: "PROJ"}},
		{"non-key passthrough", "not a key", Ref{IssueKey: "not a key"}},
		{"cloud browse", "https://acme.atlassian.net/browse/ENG-42",
			Ref{IssueKey: "ENG-42", ProjectKey: "ENG", BaseURL: "https://acme.atlassian.net",
				Flavor: FlavorCloud, IsURL: true}},
		{"dc browse with context path", "https://jira.corp.example/jira/browse/OPS-9",
			Ref{IssueKey: "OPS-9", ProjectKey: "OPS", BaseURL: "https://jira.corp.example/jira",
				IsURL: true}},
		{"board selectedIssue", "https://acme.atlassian.net/jira/software/projects/ENG/boards/1?selectedIssue=ENG-8",
			Ref{IssueKey: "ENG-8", ProjectKey: "ENG", BaseURL: "https://acme.atlassian.net",
				Flavor: FlavorCloud, IsURL: true}},
		{"project browse", "https://acme.atlassian.net/browse/ENG",
			Ref{ProjectKey: "ENG", BaseURL: "https://acme.atlassian.net",
				Flavor: FlavorCloud, IsURL: true}},
		{"projects path", "https://acme.atlassian.net/jira/software/projects/ENG/boards/1",
			Ref{ProjectKey: "ENG", BaseURL: "https://acme.atlassian.net",
				Flavor: FlavorCloud, IsURL: true}},
		{"comment permalink", "https://acme.atlassian.net/browse/ENG-42?focusedCommentId=10001",
			Ref{IssueKey: "ENG-42", ProjectKey: "ENG", CommentID: "10001",
				BaseURL: "https://acme.atlassian.net", Flavor: FlavorCloud, IsURL: true}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Parse(tc.in)
			if got != tc.want {
				t.Errorf("Parse(%q)\n got:  %+v\n want: %+v", tc.in, got, tc.want)
			}
		})
	}
}
