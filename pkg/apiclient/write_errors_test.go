package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
	"github.com/angelmsger/jira-cli/pkg/transport"
)

type writeTestDoer func(*http.Request) (*http.Response, error)

func (f writeTestDoer) Do(req *http.Request) (*http.Response, error) { return f(req) }

func writeTestClient(flavor Flavor, doer writeTestDoer) Client {
	return New(Config{Flavor: flavor, BaseURL: "https://jira.example", Transport: transport.New(transport.Options{
		Doer: doer, MaxRetries: 2, RetryBaseDelay: time.Nanosecond,
	})})
}

func writeTestResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func TestWriteSucceededReadFailed(t *testing.T) {
	for _, flavor := range []Flavor{FlavorCloud, FlavorDataCenter} {
		for _, operation := range []string{"create", "edit", "transition"} {
			t.Run(string(flavor)+"/"+operation, func(t *testing.T) {
				writes, reads := 0, 0
				client := writeTestClient(flavor, func(req *http.Request) (*http.Response, error) {
					if req.Method == http.MethodGet {
						reads++
						return writeTestResponse(403, `{"errorMessages":["cannot read issue"]}`), nil
					}
					writes++
					return writeTestResponse(201, `{"key":"ENG-123"}`), nil
				})
				var issue *Issue
				var err error
				switch operation {
				case "create":
					issue, err = client.CreateIssue(context.Background(), CreateIssueReq{ProjectKey: "ENG", Type: "Task", Summary: "s"})
				case "edit":
					summary := "updated"
					issue, err = client.EditIssue(context.Background(), EditIssueReq{Key: "ENG-123", Summary: &summary})
				case "transition":
					err = client.TransitionIssue(context.Background(), TransitionIssueReq{Key: "ENG-123", TransitionID: "21", Comment: "done"})
					if err == nil {
						issue, err = GetIssueAfterWrite(context.Background(), client, "ENG-123")
					}
				}
				ce := cerrors.AsCLIError(err)
				if ce == nil || ce.Code != "WRITE_SUCCEEDED_READ_FAILED" || ce.Retryable || ce.Category != cerrors.CategoryPermission || ce.HTTPStatus != 403 {
					t.Fatalf("unexpected error: %+v", ce)
				}
				if issue != nil || writes != 1 || reads != 1 {
					t.Fatalf("issue=%+v writes=%d reads=%d", issue, writes, reads)
				}
				if !strings.Contains(ce.Message, "ENG-123 (https://jira.example/browse/ENG-123)") || len(ce.NextSteps) != 1 || ce.NextSteps[0] != "jira-cli issue get 'ENG-123'" {
					t.Fatalf("missing identity/read recovery: %+v", ce)
				}
				var cause *cerrors.CLIError
				if !errors.As(errors.Unwrap(ce), &cause) || cause.HTTPStatus != 403 {
					t.Fatal("original read error was not preserved")
				}
			})
		}
	}
}

func TestWriteFailuresCannotBeReplayed(t *testing.T) {
	ctx := context.Background()
	summary := "updated"
	operations := map[string]func(Client) error{
		"create": func(c Client) error {
			_, err := c.CreateIssue(ctx, CreateIssueReq{ProjectKey: "ENG", Type: "Task", Summary: "s"})
			return err
		},
		"edit": func(c Client) error {
			_, err := c.EditIssue(ctx, EditIssueReq{Key: "ENG-123", Summary: &summary})
			return err
		},
		"assign": func(c Client) error { return c.AssignIssue(ctx, AssignIssueReq{Key: "ENG-123", Unassign: true}) },
		"transition": func(c Client) error {
			return c.TransitionIssue(ctx, TransitionIssueReq{Key: "ENG-123", TransitionID: "21"})
		},
		"comment add": func(c Client) error {
			_, err := c.AddComment(ctx, AddCommentReq{IssueKey: "ENG-123", Body: "text"})
			return err
		},
		"comment update": func(c Client) error {
			_, err := c.UpdateComment(ctx, UpdateCommentReq{IssueKey: "ENG-123", ID: "42", Body: "text"})
			return err
		},
		"comment delete": func(c Client) error { return c.DeleteComment(ctx, DeleteCommentReq{IssueKey: "ENG-123", ID: "42"}) },
	}
	for _, flavor := range []Flavor{FlavorCloud, FlavorDataCenter} {
		for name, operation := range operations {
			for _, status := range []int{0, 302, 429, 503} {
				t.Run(string(flavor)+"/"+name+"/"+http.StatusText(status), func(t *testing.T) {
					requests := 0
					transportErr := errors.New("connection lost after request")
					client := writeTestClient(flavor, func(req *http.Request) (*http.Response, error) {
						requests++
						if status == 0 {
							return nil, transportErr
						}
						return writeTestResponse(status, `{"errorMessages":["temporarily unavailable"]}`), nil
					})
					err := operation(client)
					ce := cerrors.AsCLIError(err)
					if ce == nil || ce.Retryable || !strings.Contains(ce.Message, "outcome is unknown") || requests != 1 || ce.HTTPStatus != status {
						t.Fatalf("requests=%d error=%+v", requests, ce)
					}
					if status == 0 && !errors.Is(err, transportErr) {
						t.Fatal("lost transport cause")
					}
					if len(ce.NextSteps) != 1 || strings.Contains(ce.NextSteps[0], "--all") || !strings.Contains(ce.NextSteps[0], "jira-cli") {
						t.Fatalf("unsafe recovery: %+v", ce)
					}
				})
			}
		}
	}
}

func TestAcknowledgedWriteInvalidResponse(t *testing.T) {
	for _, flavor := range []Flavor{FlavorCloud, FlavorDataCenter} {
		for _, body := range []string{"{broken", "{}", ""} {
			for _, operation := range []string{"create", "comment add", "comment update"} {
				t.Run(string(flavor)+"/"+operation+"/"+body, func(t *testing.T) {
					requests := 0
					client := writeTestClient(flavor, func(req *http.Request) (*http.Response, error) {
						requests++
						return writeTestResponse(201, body), nil
					})
					var err error
					ctx := context.Background()
					switch operation {
					case "create":
						_, err = client.CreateIssue(ctx, CreateIssueReq{ProjectKey: "ENG", Type: "Task", Summary: "s"})
					case "comment add":
						_, err = client.AddComment(ctx, AddCommentReq{IssueKey: "ENG-123", Body: "text"})
					case "comment update":
						_, err = client.UpdateComment(ctx, UpdateCommentReq{IssueKey: "ENG-123", ID: "42", Body: "text"})
					}
					ce := cerrors.AsCLIError(err)
					if ce == nil || ce.Code != "WRITE_SUCCEEDED_RESPONSE_INVALID" || ce.Category != cerrors.CategoryParse || ce.Retryable || requests != 1 {
						t.Fatalf("requests=%d error=%+v", requests, ce)
					}
					if body == "{broken" && ce.HTTPStatus != http.StatusCreated {
						t.Fatalf("decode error lost acknowledged HTTP status: %+v", ce)
					}
					if operation == "create" && !strings.Contains(ce.NextSteps[0], "--project 'ENG' --order-by 'created DESC' --limit 25") {
						t.Fatalf("unbounded creation recovery: %+v", ce)
					}
				})
			}
		}
	}
}

func TestWriteReadFailurePayload(t *testing.T) {
	transportErr := errors.New("connection lost")
	client := writeTestClient(FlavorCloud, func(req *http.Request) (*http.Response, error) { return nil, transportErr })
	_, err := GetIssueAfterWrite(context.Background(), client, "ENG-123")
	if !errors.Is(err, transportErr) {
		t.Fatal("lost read failure cause")
	}
	ce := cerrors.AsCLIError(err)
	data, err := json.Marshal(ce.Payload())
	if err != nil {
		t.Fatal(err)
	}
	var payload cerrors.Payload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error.Retryable || payload.Error.Code != "WRITE_SUCCEEDED_READ_FAILED" || payload.Error.Category != cerrors.CategoryNetwork || !strings.Contains(payload.Error.Message, "https://jira.example/browse/ENG-123") {
		t.Fatalf("payload=%s", data)
	}
}

func TestReadFailureRecoveryUnchanged(t *testing.T) {
	for _, flavor := range []Flavor{FlavorCloud, FlavorDataCenter} {
		t.Run(string(flavor), func(t *testing.T) {
			client := writeTestClient(flavor, func(req *http.Request) (*http.Response, error) {
				return writeTestResponse(503, `{"errorMessages":["temporarily unavailable"]}`), nil
			})
			_, err := client.SearchIssues(context.Background(), SearchOpts{JQL: "project=ENG"})
			ce := cerrors.AsCLIError(err)
			if ce == nil || !ce.Retryable || ce.Category != cerrors.CategoryServer || strings.Contains(ce.Message, "Write") {
				t.Fatalf("read recovery changed: %+v", ce)
			}
		})
	}
}
