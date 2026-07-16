package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/angelmsger/jira-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
	"github.com/angelmsger/jira-cli/pkg/transport"
	"github.com/angelmsger/jira-cli/pkg/urlref"
	"github.com/spf13/cobra"
)

// buildProbeTransport returns an unauthenticated transport used for flavor
// detection and connectivity checks.
func buildProbeTransport(s *appState) *transport.Client {
	return transport.New(transport.Options{
		Timeout:    s.timeout(),
		MaxRetries: s.cfg().Defaults.MaxRetries,
	})
}

// cmdContext returns a context bounded by the configured request timeout.
func cmdContext(s *appState) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), s.timeout())
}

// resolveIssueKey extracts a Jira issue key from a bare key (PROJ-123), a
// /browse URL, or a board URL carrying ?selectedIssue=.
func resolveIssueKey(arg string) (string, error) {
	ref := urlref.Parse(arg)
	if ref.IssueKey == "" {
		return "", cerrors.Newf(cerrors.CategoryUsage, "NO_ISSUE_KEY",
			"could not resolve an issue key from %q", arg).
			WithHint("Pass an issue key like PROJ-123 or a Jira issue URL.").
			WithNextSteps("jira-cli issue search --text \"<keywords>\" to find the issue key.")
	}
	return ref.IssueKey, nil
}

// dryRunOutput is the result shape emitted for a --dry-run write.
type dryRunOutput struct {
	DryRun  bool   `json:"dry_run"`
	Method  string `json:"method"`
	URL     string `json:"url"`
	Payload any    `json:"payload,omitempty"`
}

// emitDryRun resolves a write request into the HTTP request it would send and
// emits that plan instead of performing the write.
func emitDryRun(s *appState, client apiclient.Client, ctx context.Context, op any) error {
	out, err := dryRunPlan(client, ctx, op)
	if err != nil {
		return err
	}
	return s.emit(out)
}

// dryRunPlan resolves a write request into its dry-run output object without
// emitting it, so batch commands can collect one plan per item.
func dryRunPlan(client apiclient.Client, ctx context.Context, op any) (dryRunOutput, error) {
	plan, err := client.DescribeWrite(ctx, op)
	if err != nil {
		return dryRunOutput{}, err
	}
	return dryRunOutput{
		DryRun: true, Method: plan.Method, URL: plan.URL, Payload: plan.Payload,
	}, nil
}

// confirmDelete gates a destructive operation: --yes skips it, an interactive
// terminal is prompted, and a non-TTY without --yes fails with a structured
// usage error so agents get an explicit signal instead of a hang.
func confirmDelete(prompt string, yes bool) error {
	if yes {
		return nil
	}
	if !stdinIsTTY() {
		return cerrors.New(cerrors.CategoryUsage, "DELETE_NEEDS_YES",
			"delete requires --yes when stdin is not a terminal").
			WithHint("Re-run with --yes to confirm the deletion.")
	}
	fmt.Fprintf(os.Stderr, "Delete %s? [y/N] ", prompt)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	if ans := strings.ToLower(strings.TrimSpace(line)); ans != "y" && ans != "yes" {
		return cerrors.New(cerrors.CategoryUsage, "DELETE_ABORTED", "deletion cancelled")
	}
	return nil
}

// readBodyText resolves text for a body-bearing flag pair (--body/--body-file
// or --description/--description-file). errCode names the command's noun so
// the failure is precise.
func readBodyText(inline, file, errCode string) (string, error) {
	if inline != "" {
		return inline, nil
	}
	if file == "" {
		return "", cerrors.New(cerrors.CategoryUsage, errCode,
			"provide the text inline or via the *-file flag")
	}
	var raw []byte
	var err error
	if file == "-" {
		raw, err = io.ReadAll(os.Stdin)
	} else {
		raw, err = os.ReadFile(file)
	}
	if err != nil {
		return "", cerrors.Wrap(err, cerrors.CategoryUsage, errCode+"_READ",
			"failed to read the text file")
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "", cerrors.New(cerrors.CategoryUsage, errCode, "the text is empty")
	}
	return text, nil
}

// addListFlags registers the shared pagination flags every list command takes.
func addListFlags(cmd *cobra.Command, limit *int, all *bool, cursor *string) {
	f := cmd.Flags()
	f.IntVar(limit, "limit", 0, "page size (default from config)")
	f.BoolVar(all, "all", false, "fetch every page of results")
	f.StringVar(cursor, "cursor", "", "start from this pagination cursor (the 'next' of a prior page)")
}

// pageInfo carries the pagination cursor for one page of a listing.
type pageInfo struct {
	Next    string
	HasMore bool
}

// collectPage fetches results for a list command. With all set it walks every
// page starting at cursor and returns the full set; otherwise it returns the
// single page at cursor plus the cursor for the page after it.
func collectPage[T any](fetch apiclient.FetchPage[T], cursor string, all bool) ([]T, pageInfo, error) {
	if all {
		var items []T
		c := cursor
		for {
			page, err := fetch(c)
			if err != nil {
				return items, pageInfo{}, err
			}
			items = append(items, page.Items...)
			if page.Next == "" {
				return items, pageInfo{}, nil
			}
			c = page.Next
		}
	}
	page, err := fetch(cursor)
	if err != nil {
		return nil, pageInfo{}, err
	}
	return page.Items, pageInfo{Next: page.Next, HasMore: page.Next != ""}, nil
}
