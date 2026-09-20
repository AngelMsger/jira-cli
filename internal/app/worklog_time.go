package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
)

// worklog_time.go absorbs Jira's time-handling footguns and implements the
// family's event-window contract (docs/agent-facing-cli-best-practices.md
// §3.5), matching confluence-cli's page_history_filter.go and bitbucket-cli's
// activity_filter.go so the three CLIs behave and read alike.

// jiraInstantLayout is Jira's worklog `started` wire format: millisecond
// precision and a colon-free numeric offset. It is deliberately NOT RFC 3339 —
// sending an RFC 3339 string is rejected by the API.
const jiraInstantLayout = "2006-01-02T15:04:05.000-0700"

// formatJiraInstant renders an instant in Jira's wire format.
func formatJiraInstant(t time.Time) string { return t.Format(jiraInstantLayout) }

// worklogWindow is a half-open [from, to) interval. Adjacent windows therefore
// never overlap, so an agent can page through time without double-counting.
type worklogWindow struct {
	from time.Time
	to   time.Time
	set  bool
}

// contains reports whether an instant falls inside the half-open window.
func (w worklogWindow) contains(t time.Time) bool {
	if !w.set {
		return true
	}
	return !t.Before(w.from) && t.Before(w.to)
}

// resolveWorklogWindow turns --since / --from / --to into a half-open interval.
func resolveWorklogWindow(since, from, to string, now time.Time) (worklogWindow, error) {
	if since != "" && (from != "" || to != "") {
		return worklogWindow{}, badTimeRange("--since cannot be combined with --from or --to")
	}
	if to != "" && from == "" {
		return worklogWindow{}, badTimeRange("--to requires --from")
	}
	if since == "" && from == "" {
		return worklogWindow{}, nil
	}
	if now.IsZero() {
		now = time.Now()
	}

	var start, end time.Time
	var err error
	if since != "" {
		duration, derr := parseWorklogDuration(since)
		if derr != nil || duration <= 0 {
			return worklogWindow{}, badTimeRange(
				fmt.Sprintf("invalid --since %q: use a positive duration such as 24h or 7d", since))
		}
		start, end = now.Add(-duration), now
	} else {
		start, err = parseWindowInstant(from)
		if err != nil {
			return worklogWindow{}, badTimeRange(fmt.Sprintf("invalid --from %q: %v", from, err))
		}
		end = now
		if to != "" {
			end, err = parseWindowInstant(to)
			if err != nil {
				return worklogWindow{}, badTimeRange(fmt.Sprintf("invalid --to %q: %v", to, err))
			}
		}
	}
	if !end.After(start) {
		return worklogWindow{}, badTimeRange("--to must be later than --from")
	}
	return worklogWindow{from: start, to: end, set: true}, nil
}

func badTimeRange(msg string) error {
	return cerrors.New(cerrors.CategoryUsage, "BAD_TIME_RANGE", msg).
		WithHint("--from is inclusive and --to is exclusive. Date-only values are UTC; " +
			"use an RFC3339 instant with an explicit offset for a local calendar day.")
}

// parseWorklogDuration accepts Go duration syntax plus the 'd' and 'w' suffixes
// the family's other time-window flags accept.
func parseWorklogDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if len(value) > 1 {
		n, err := strconv.Atoi(value[:len(value)-1])
		if err == nil {
			switch value[len(value)-1] {
			case 'd':
				return time.Duration(n) * 24 * time.Hour, nil
			case 'w':
				return time.Duration(n) * 7 * 24 * time.Hour, nil
			}
		}
	}
	return time.ParseDuration(value)
}

// windowLayouts are the offset-bearing instant formats the window flags accept.
// The last two are Jira's own colon-free variants, so a timestamp copied out of
// an API response can be pasted straight back in.
var windowLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05.000-0700",
	"2006-01-02T15:04:05-0700",
}

// parseWindowInstant parses a --from / --to value. Date-only values are UTC,
// per the family's shared contract.
func parseWindowInstant(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	for _, layout := range windowLayouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	if parsed, err := time.ParseInLocation("2006-01-02", value, time.UTC); err == nil {
		return parsed, nil
	}
	return time.Time{}, fmt.Errorf("use RFC3339 or a date in YYYY-MM-DD form (date-only values are UTC)")
}

// parseStartedInstant parses --started. It accepts everything the window flags
// accept, plus "now" and an offset-less wall clock, which is read in the local
// zone — when someone types the time they started working, they mean their own
// clock.
func parseStartedInstant(value string, now time.Time) (time.Time, error) {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "now") {
		if now.IsZero() {
			now = time.Now()
		}
		return now, nil
	}
	for _, layout := range windowLayouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02 15:04"} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed, nil
		}
	}
	if parsed, err := time.ParseInLocation("2006-01-02", value, time.UTC); err == nil {
		return parsed, nil
	}
	return time.Time{}, cerrors.Newf(cerrors.CategoryUsage, "WORKLOG_BAD_STARTED",
		"invalid --started %q", value).
		WithHint(`Use "now", an RFC3339 instant with an offset, a local wall clock ` +
			`("2006-01-02T15:04:05"), or a date ("2006-01-02", read as UTC midnight).`)
}

// resolveStarted turns a --started flag into Jira's wire format. An empty value
// omits the field, letting Jira default to now.
func resolveStarted(value string, now time.Time) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	parsed, err := parseStartedInstant(value, now)
	if err != nil {
		return "", err
	}
	return formatJiraInstant(parsed), nil
}
