package apiclient

import (
	"context"
	"net/url"

	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
)

// rawTransition is the wire shape of a workflow transition (same on both
// flavors).
type rawTransition struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	To   *struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		StatusCategory struct {
			Key string `json:"key"`
		} `json:"statusCategory"`
	} `json:"to"`
}

func mapTransition(r rawTransition) Transition {
	t := Transition{ID: r.ID, Name: r.Name}
	if r.To != nil {
		t.To = &Status{
			ID:       r.To.ID,
			Name:     r.To.Name,
			Category: r.To.StatusCategory.Key,
		}
	}
	return t
}

// ListTransitions lists the workflow transitions currently available on an
// issue. The set depends on the issue's status and the caller's permissions.
//
//	Both flavors: GET /issue/{key}/transitions
func (c *apiClient) ListTransitions(ctx context.Context, issueKey string) (ListResult[Transition], error) {
	if issueKey == "" {
		return ListResult[Transition]{}, cerrors.New(cerrors.CategoryUsage, "ISSUE_NO_KEY",
			"an issue key is required")
	}
	var raw struct {
		Transitions []rawTransition `json:"transitions"`
	}
	path := c.apiBase() + "/issue/" + url.PathEscape(issueKey) + "/transitions"
	if err := c.getJSON(ctx, path, nil, &raw); err != nil {
		return ListResult[Transition]{}, err
	}
	res := ListResult[Transition]{}
	for _, r := range raw.Transitions {
		res.Items = append(res.Items, mapTransition(r))
	}
	return res, nil
}
