package apiclient

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
)

// worklogs_write.go holds the worklog write operations. Each has a build*
// helper shared with DescribeWrite so --dry-run previews the exact request,
// including the estimate-adjustment query string.

// jiraDurationPattern matches Jira's own time grammar: whitespace-separated
// magnitude+unit pairs, such as "3h 20m", "1d 4h" or "45m". Jira expands w/d
// using the instance's time-tracking configuration, so the CLI validates the
// shape and forwards the value verbatim rather than converting it.
var jiraDurationPattern = regexp.MustCompile(`^\d+(\.\d+)?[wdhm](\s+\d+(\.\d+)?[wdhm])*$`)

// validateJiraDuration rejects a malformed duration before any HTTP round trip.
// flag names the flag in the error so the message is actionable.
func validateJiraDuration(value, flag string) error {
	if !jiraDurationPattern.MatchString(strings.ToLower(strings.TrimSpace(value))) {
		return cerrors.Newf(cerrors.CategoryUsage, "WORKLOG_BAD_TIME_SPENT",
			"%s value %q is not a Jira duration", flag, value).
			WithHint(`Use Jira's units: a magnitude and one of w, d, h or m, ` +
				`space-separated for several — for example "3h 20m", "1d 4h" or "45m".`)
	}
	return nil
}

// estimateQuery validates an EstimateAdjust for one operation and renders it as
// query parameters. op is "add", "update" or "delete"; an empty Mode sends
// nothing, leaving Jira's own default (auto) in force.
//
// Jira's PUT .../worklog/{id} accepts only adjustEstimate + newEstimate, so
// "manual" — which needs reduceBy / increaseBy — is unavailable on update.
func estimateQuery(op string, e EstimateAdjust) (url.Values, error) {
	mode := strings.ToLower(strings.TrimSpace(e.Mode))
	companion := func(name, value string) error {
		if value == "" {
			return nil
		}
		return cerrors.Newf(cerrors.CategoryUsage, "WORKLOG_ESTIMATE_FLAG_UNSUPPORTED",
			"--%s is not valid with --adjust-estimate %q", name, orAuto(mode)).
			WithHint("--new-estimate goes with --adjust-estimate new; " +
				"--reduce-by and --increase-by go with --adjust-estimate manual.")
	}

	if mode == "" {
		for _, c := range companions(e) {
			if c.value != "" {
				return nil, cerrors.Newf(cerrors.CategoryUsage, "WORKLOG_ESTIMATE_FLAG_UNSUPPORTED",
					"--%s requires --adjust-estimate", c.name).
					WithNextSteps("Pass --adjust-estimate new with --new-estimate, " +
						"or --adjust-estimate manual with --reduce-by / --increase-by.")
			}
		}
		return nil, nil
	}

	q := url.Values{}
	switch mode {
	case "auto", "leave":
		for _, c := range companions(e) {
			if err := companion(c.name, c.value); err != nil {
				return nil, err
			}
		}
	case "new":
		if e.NewEstimate == "" {
			return nil, missingEstimateFlag("new-estimate", "new")
		}
		if err := validateJiraDuration(e.NewEstimate, "--new-estimate"); err != nil {
			return nil, err
		}
		if err := companion("reduce-by", e.ReduceBy); err != nil {
			return nil, err
		}
		if err := companion("increase-by", e.IncreaseBy); err != nil {
			return nil, err
		}
		q.Set("newEstimate", e.NewEstimate)
	case "manual":
		if err := companion("new-estimate", e.NewEstimate); err != nil {
			return nil, err
		}
		switch op {
		case "add":
			if err := companion("increase-by", e.IncreaseBy); err != nil {
				return nil, err
			}
			if e.ReduceBy == "" {
				return nil, missingEstimateFlag("reduce-by", "manual")
			}
			if err := validateJiraDuration(e.ReduceBy, "--reduce-by"); err != nil {
				return nil, err
			}
			q.Set("reduceBy", e.ReduceBy)
		case "delete":
			if err := companion("reduce-by", e.ReduceBy); err != nil {
				return nil, err
			}
			if e.IncreaseBy == "" {
				return nil, missingEstimateFlag("increase-by", "manual")
			}
			if err := validateJiraDuration(e.IncreaseBy, "--increase-by"); err != nil {
				return nil, err
			}
			q.Set("increaseBy", e.IncreaseBy)
		default: // update
			return nil, cerrors.New(cerrors.CategoryUsage, "WORKLOG_ESTIMATE_FLAG_UNSUPPORTED",
				`--adjust-estimate manual is not supported when updating a worklog`).
				WithHint("Jira's worklog update endpoint accepts only adjustEstimate " +
					"new, leave or auto.").
				WithNextSteps("Use --adjust-estimate new --new-estimate <duration> to set " +
					"the remaining estimate explicitly.")
		}
	default:
		return nil, cerrors.Newf(cerrors.CategoryUsage, "WORKLOG_BAD_ADJUST_ESTIMATE",
			"unknown --adjust-estimate %q", e.Mode).
			WithHint(`Valid values are "auto" (subtract the logged time), "new" ` +
				`(set --new-estimate), "leave" (unchanged) and "manual" ` +
				`(--reduce-by / --increase-by).`)
	}
	q.Set("adjustEstimate", mode)
	return q, nil
}

// companions lists the estimate companion flags in a fixed order, so an
// invalid combination always reports the same flag rather than whichever one
// a map happened to yield first.
func companions(e EstimateAdjust) []struct{ name, value string } {
	return []struct{ name, value string }{
		{"new-estimate", e.NewEstimate},
		{"reduce-by", e.ReduceBy},
		{"increase-by", e.IncreaseBy},
	}
}

func missingEstimateFlag(flag, mode string) error {
	return cerrors.Newf(cerrors.CategoryUsage, "WORKLOG_ESTIMATE_FLAG_MISSING",
		"--adjust-estimate %s requires --%s", mode, flag)
}

func orAuto(mode string) string {
	if mode == "" {
		return "auto"
	}
	return mode
}

// withQuery appends encoded query parameters to a path. The estimate
// parameters ride on the path so DescribeWrite's URL — built as baseURL+path —
// previews exactly what the live request sends.
func withQuery(path string, q url.Values) string {
	if len(q) == 0 {
		return path
	}
	return path + "?" + q.Encode()
}

// timeSpentPayload validates the mutually exclusive time inputs and renders
// them into the request body. required reports whether one of them must be set.
func timeSpentPayload(spent string, seconds int64, required bool, payload map[string]any) error {
	switch {
	case seconds < 0:
		return cerrors.New(cerrors.CategoryUsage, "WORKLOG_BAD_TIME_SPENT",
			"--time-spent-seconds must be positive")
	case spent != "" && seconds > 0:
		return cerrors.New(cerrors.CategoryUsage, "WORKLOG_TIME_SPENT_CONFLICT",
			"--time-spent and --time-spent-seconds are mutually exclusive")
	case spent != "":
		if err := validateJiraDuration(spent, "--time-spent"); err != nil {
			return err
		}
		payload["timeSpent"] = spent
	case seconds > 0:
		payload["timeSpentSeconds"] = seconds
	case required:
		return cerrors.New(cerrors.CategoryUsage, "WORKLOG_NO_TIME_SPENT",
			"logging work requires --time-spent or --time-spent-seconds").
			WithHint(`--time-spent takes Jira's units, for example "3h 20m".`)
	}
	return nil
}

// buildAddWorklog assembles the POST request for logging work.
func (c *apiClient) buildAddWorklog(req AddWorklogReq) (method, path string, payload any, err error) {
	if req.IssueKey == "" {
		return "", "", nil, cerrors.New(cerrors.CategoryUsage, "ISSUE_NO_KEY",
			"an issue key is required to log work")
	}
	body := map[string]any{}
	if err := timeSpentPayload(req.TimeSpent, req.TimeSpentSeconds, true, body); err != nil {
		return "", "", nil, err
	}
	if req.Started != "" {
		body["started"] = req.Started
	}
	if req.Comment != "" {
		body["comment"] = c.textToBody(req.Comment)
	}
	q, err := estimateQuery("add", req.Estimate)
	if err != nil {
		return "", "", nil, err
	}
	path = c.apiBase() + "/issue/" + url.PathEscape(req.IssueKey) + "/worklog"
	return "POST", withQuery(path, q), body, nil
}

// AddWorklog logs work on an issue.
func (c *apiClient) AddWorklog(ctx context.Context, req AddWorklogReq) (*Worklog, error) {
	method, path, payload, err := c.buildAddWorklog(req)
	if err != nil {
		return nil, err
	}
	var raw rawWorklog
	target := c.worklogWriteTarget(req.IssueKey, "")
	if err := c.doWriteJSON(ctx, method, path, payload, &raw, target); err != nil {
		return nil, err
	}
	if raw.ID == "" {
		return nil, missingWriteIdentity(target)
	}
	out := mapWorklog(raw, req.IssueKey)
	return &out, nil
}

// buildUpdateWorklog assembles the PUT request for changing a worklog.
func (c *apiClient) buildUpdateWorklog(req UpdateWorklogReq) (method, path string, payload any, err error) {
	if req.IssueKey == "" {
		return "", "", nil, cerrors.New(cerrors.CategoryUsage, "ISSUE_NO_KEY",
			"an issue key is required to update a worklog")
	}
	if req.ID == "" {
		return "", "", nil, worklogNoID("update")
	}
	body := map[string]any{}
	if err := timeSpentPayload(req.TimeSpent, req.TimeSpentSeconds, false, body); err != nil {
		return "", "", nil, err
	}
	if req.Started != "" {
		body["started"] = req.Started
	}
	if req.Comment != "" {
		body["comment"] = c.textToBody(req.Comment)
	}
	q, err := estimateQuery("update", req.Estimate)
	if err != nil {
		return "", "", nil, err
	}
	if len(body) == 0 {
		return "", "", nil, cerrors.New(cerrors.CategoryUsage, "WORKLOG_NO_UPDATE_FIELDS",
			"updating a worklog requires at least one change").
			WithNextSteps("Pass --time-spent, --time-spent-seconds, --started, " +
				"--comment or --comment-file.")
	}
	path = c.apiBase() + "/issue/" + url.PathEscape(req.IssueKey) + "/worklog/" + url.PathEscape(req.ID)
	return "PUT", withQuery(path, q), body, nil
}

// UpdateWorklog changes an existing worklog.
func (c *apiClient) UpdateWorklog(ctx context.Context, req UpdateWorklogReq) (*Worklog, error) {
	method, path, payload, err := c.buildUpdateWorklog(req)
	if err != nil {
		return nil, err
	}
	var raw rawWorklog
	target := c.worklogWriteTarget(req.IssueKey, req.ID)
	if err := c.doWriteJSON(ctx, method, path, payload, &raw, target); err != nil {
		return nil, err
	}
	if raw.ID == "" {
		return nil, missingWriteIdentity(target)
	}
	out := mapWorklog(raw, req.IssueKey)
	return &out, nil
}

// buildDeleteWorklog assembles the DELETE request for a worklog.
func (c *apiClient) buildDeleteWorklog(req DeleteWorklogReq) (method, path string, err error) {
	if req.IssueKey == "" {
		return "", "", cerrors.New(cerrors.CategoryUsage, "ISSUE_NO_KEY",
			"an issue key is required to delete a worklog")
	}
	if req.ID == "" {
		return "", "", worklogNoID("delete")
	}
	q, qerr := estimateQuery("delete", req.Estimate)
	if qerr != nil {
		return "", "", qerr
	}
	path = c.apiBase() + "/issue/" + url.PathEscape(req.IssueKey) + "/worklog/" + url.PathEscape(req.ID)
	return "DELETE", withQuery(path, q), nil
}

// DeleteWorklog deletes a worklog.
func (c *apiClient) DeleteWorklog(ctx context.Context, req DeleteWorklogReq) error {
	method, path, err := c.buildDeleteWorklog(req)
	if err != nil {
		return err
	}
	return c.doWriteJSON(ctx, method, path, nil, nil, c.worklogWriteTarget(req.IssueKey, req.ID))
}

// worklogNoID reports a missing worklog ID and names the command that discovers
// one, so the caller is never left at a dead end.
func worklogNoID(verb string) error {
	return cerrors.Newf(cerrors.CategoryUsage, "WORKLOG_NO_ID",
		"a worklog ID is required to %s a worklog", verb).
		WithNextSteps("jira-cli worklog list <issue-key> --limit 25")
}
