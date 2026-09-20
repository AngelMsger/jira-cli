package app

import (
	"testing"
	"time"

	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
)

// fixedNow is the reference instant the window tests resolve --since against.
var fixedNow = time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)

// TestResolveWorklogWindow pins the family's event-window contract: --since is
// exclusive with --from/--to, --to requires --from, and the interval is always
// half-open so adjacent windows never overlap.
func TestResolveWorklogWindow(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		since    string
		from     string
		to       string
		wantSet  bool
		wantFrom time.Time
		wantTo   time.Time
		wantErr  bool
	}{
		{name: "unset", wantSet: false},
		{
			name: "since hours", since: "24h", wantSet: true,
			wantFrom: fixedNow.Add(-24 * time.Hour), wantTo: fixedNow,
		},
		{
			name: "since days", since: "7d", wantSet: true,
			wantFrom: fixedNow.AddDate(0, 0, -7), wantTo: fixedNow,
		},
		{
			name: "since weeks", since: "2w", wantSet: true,
			wantFrom: fixedNow.AddDate(0, 0, -14), wantTo: fixedNow,
		},
		{
			name: "from defaults to now", from: "2026-09-01", wantSet: true,
			wantFrom: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), wantTo: fixedNow,
		},
		{
			name: "date-only values are UTC", from: "2026-09-01", to: "2026-09-08", wantSet: true,
			wantFrom: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
			wantTo:   time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "rfc3339 with an offset", from: "2026-09-01T00:00:00+08:00", wantSet: true,
			wantFrom: time.Date(2026, 8, 31, 16, 0, 0, 0, time.UTC), wantTo: fixedNow,
		},
		{
			// Jira's own colon-free layout, so a timestamp copied out of a
			// response can be pasted straight back in.
			name: "jira instant layout", from: "2026-09-01T00:00:00.000+0800", wantSet: true,
			wantFrom: time.Date(2026, 8, 31, 16, 0, 0, 0, time.UTC), wantTo: fixedNow,
		},

		{name: "since with from", since: "7d", from: "2026-09-01", wantErr: true},
		{name: "since with to", since: "7d", to: "2026-09-08", wantErr: true},
		{name: "to without from", to: "2026-09-08", wantErr: true},
		{name: "inverted range", from: "2026-09-08", to: "2026-09-01", wantErr: true},
		{name: "empty range", from: "2026-09-01", to: "2026-09-01", wantErr: true},
		{name: "negative since", since: "-7d", wantErr: true},
		{name: "unparseable since", since: "last week", wantErr: true},
		{name: "unparseable from", since: "", from: "01/09/2026", wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveWorklogWindow(tc.since, tc.from, tc.to, fixedNow)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveWorklogWindow = %+v, want an error", got)
				}
				if code := cerrors.AsCLIError(err).Code; code != "BAD_TIME_RANGE" {
					t.Errorf("code = %s, want BAD_TIME_RANGE", code)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveWorklogWindow: %v", err)
			}
			if got.set != tc.wantSet {
				t.Fatalf("set = %v, want %v", got.set, tc.wantSet)
			}
			if !tc.wantSet {
				return
			}
			if !got.from.Equal(tc.wantFrom) {
				t.Errorf("from = %s, want %s", got.from, tc.wantFrom)
			}
			if !got.to.Equal(tc.wantTo) {
				t.Errorf("to = %s, want %s", got.to, tc.wantTo)
			}
		})
	}
}

// TestWorklogWindowIsHalfOpen proves the [from, to) boundary directly: an
// instant exactly on `to` belongs to the *next* window, not this one.
func TestWorklogWindowIsHalfOpen(t *testing.T) {
	t.Parallel()
	w, err := resolveWorklogWindow("", "2026-09-01", "2026-09-08", fixedNow)
	if err != nil {
		t.Fatalf("resolveWorklogWindow: %v", err)
	}
	cases := []struct {
		instant string
		want    bool
	}{
		{"2026-08-31T23:59:59Z", false},
		{"2026-09-01T00:00:00Z", true}, // from is inclusive
		{"2026-09-07T23:59:59Z", true},
		{"2026-09-08T00:00:00Z", false}, // to is exclusive
	}
	for _, tc := range cases {
		at, err := time.Parse(time.RFC3339, tc.instant)
		if err != nil {
			t.Fatalf("parse %s: %v", tc.instant, err)
		}
		if got := w.contains(at); got != tc.want {
			t.Errorf("contains(%s) = %v, want %v", tc.instant, got, tc.want)
		}
	}
	unset := worklogWindow{}
	if !unset.contains(fixedNow) {
		t.Error("an unset window should not filter anything out")
	}
}

// TestResolveStarted covers --started end to end: every accepted input layout
// down to Jira's colon-free wire format, which is not RFC 3339.
// It is deliberately NOT parallel: it pins time.Local, and Go runs serial tests
// to completion before resuming any parallel one, so nothing else can observe
// the swapped zone.
func TestResolveStarted(t *testing.T) {
	local := time.FixedZone("UTC+8", 8*60*60)
	prev := time.Local
	time.Local = local
	t.Cleanup(func() { time.Local = prev })

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty omits the field", input: "", want: ""},
		{name: "now", input: "now", want: "2026-09-19T12:00:00.000+0000"},
		{name: "now is case-insensitive", input: "NOW", want: "2026-09-19T12:00:00.000+0000"},
		{name: "rfc3339 with an offset", input: "2026-09-19T09:00:00+08:00", want: "2026-09-19T09:00:00.000+0800"},
		{name: "rfc3339 utc", input: "2026-09-19T09:00:00Z", want: "2026-09-19T09:00:00.000+0000"},
		{name: "jira wire format round-trips", input: "2026-09-19T09:00:00.000+0800", want: "2026-09-19T09:00:00.000+0800"},
		{name: "local wall clock", input: "2026-09-19T09:00:00", want: "2026-09-19T09:00:00.000+0800"},
		{name: "local wall clock without seconds", input: "2026-09-19T09:00", want: "2026-09-19T09:00:00.000+0800"},
		{name: "space-separated wall clock", input: "2026-09-19 09:00:00", want: "2026-09-19T09:00:00.000+0800"},
		{name: "date-only is utc midnight", input: "2026-09-19", want: "2026-09-19T00:00:00.000+0000"},
		{name: "surrounding whitespace", input: "  now  ", want: "2026-09-19T12:00:00.000+0000"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveStarted(tc.input, fixedNow)
			if err != nil {
				t.Fatalf("resolveStarted(%q): %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("resolveStarted(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestResolveStartedRejectsGarbage keeps an unparseable instant from reaching
// Jira, which would answer with an opaque 400.
func TestResolveStartedRejectsGarbage(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"yesterday", "19/09/2026", "2026-13-01", "1758240000"} {
		_, err := resolveStarted(input, fixedNow)
		if err == nil {
			t.Errorf("resolveStarted(%q) = nil, want an error", input)
			continue
		}
		if code := cerrors.AsCLIError(err).Code; code != "WORKLOG_BAD_STARTED" {
			t.Errorf("resolveStarted(%q) code = %s, want WORKLOG_BAD_STARTED", input, code)
		}
	}
}
