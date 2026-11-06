package apiclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/angelmsger/agent-cli-kit/doctor"
	"github.com/angelmsger/agent-cli-kit/transport"
	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
)

// A read's redirect that comes back is reported with the kit's redirect rule:
// not retryable, a hint naming where it points (without its query) and
// --base-url, and doctor first. doctor reports it as redirected, not
// unreachable. The POST search's 302 is never followed as a GET; the GET's
// loop ends at the tenth hop instead of failing as a network error.
func TestReadRedirectIsReportedNotRetried(t *testing.T) {
	cases := []struct {
		name string
		read func(Client) error
		loop bool
	}{
		{"search 302", func(c Client) error {
			_, err := c.SearchIssues(context.Background(), SearchOpts{JQL: "project=ENG"})
			return err
		}, false},
		{"ping loop", func(c Client) error {
			_, err := c.Ping(context.Background())
			return err
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sent atomic.Int64
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sent.Add(1)
				if tc.loop {
					http.Redirect(w, r, r.URL.Path, http.StatusFound)
					return
				}
				http.Redirect(w, r, "https://sso.example.test/login?ticket=sign-in-secret", http.StatusFound)
			}))
			t.Cleanup(srv.Close)
			client := New(Config{
				Flavor:    FlavorCloud,
				BaseURL:   srv.URL,
				Transport: transport.New(transport.Options{MaxRetries: 2}),
			})
			err := tc.read(client)
			ce := cerrors.AsCLIError(err)
			if ce == nil || ce.Code != "HTTP_Found" || ce.HTTPStatus != http.StatusFound || ce.Retryable {
				t.Fatalf("got %+v; want HTTP_Found 302, not retryable", ce)
			}
			if !strings.Contains(ce.Hint, "check --base-url first") || strings.Contains(ce.Hint, "sign-in-secret") {
				t.Fatalf("hint = %q", ce.Hint)
			}
			if !tc.loop && !strings.Contains(ce.Hint, "redirected the request to https://sso.example.test:") {
				t.Fatalf("hint does not name the target: %q", ce.Hint)
			}
			if len(ce.NextSteps) == 0 || ce.NextSteps[0] != "jira-cli doctor" {
				t.Fatalf("next steps = %v", ce.NextSteps)
			}
			want := int64(1)
			if tc.loop {
				want = 10
			}
			if got := sent.Load(); got != want {
				t.Fatalf("requests = %d, want %d (no retry)", got, want)
			}
			if got := doctor.Status(err); got != doctor.StatusRedirected {
				t.Fatalf("doctor.Status = %q, want redirected", got)
			}
			if got := doctor.Detail(err); !strings.Contains(got, "Jira returned HTTP 302") || !strings.Contains(got, "check --base-url first") {
				t.Fatalf("doctor.Detail = %q", got)
			}
		})
	}
}
