package apiclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/angelmsger/jira-cli/pkg/transport"
)

func TestIssueURLKeepsCredentialsOutOfOutput(t *testing.T) {
	for _, tc := range []struct {
		flavor Flavor
		path   string
	}{
		{FlavorCloud, "/jira/rest/api/3/issue/ENG-1"},
		{FlavorDataCenter, "/jira/rest/api/2/issue/ENG-1"},
	} {
		t.Run(string(tc.flavor), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					t.Errorf("path = %q, want %q", r.URL.Path, tc.path)
				}
				if user, password, ok := r.BasicAuth(); !ok || user != "reader" || password != "url-secret" {
					t.Error("request lost its URL credentials")
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"id":"1","key":"ENG-1","fields":{"summary":"Welcome"}}`)
			}))
			defer server.Close()
			client := New(Config{
				Flavor:    tc.flavor,
				BaseURL:   strings.Replace(server.URL, "http://", "http://reader:url-secret@", 1) + "/jira",
				Transport: transport.New(transport.Options{}),
			})
			issue, err := client.GetIssue(context.Background(), GetIssueOpts{Key: "ENG-1"})
			if err != nil {
				t.Fatal(err)
			}
			if want := server.URL + "/jira/browse/ENG-1"; issue.URL != want {
				t.Errorf("issue URL = %q, want %q", issue.URL, want)
			}
		})
	}
}
