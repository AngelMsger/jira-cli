package apiclient

import (
	"strings"

	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
)

// JQLParams describes a search expressed as discrete filters. BuildJQL turns
// it into a Jira Query Language string.
type JQLParams struct {
	Project  string // project key (project = "...")
	Assignee string // assignee; "me" = currentUser(), "unassigned" = EMPTY
	Reporter string // reporter; "me" = currentUser()
	Status   string // status name (status = "...")
	Type     string // issue type name (issuetype = "...")
	Label    string // label (labels = "...")
	Text     string // free-text match (text ~ "...")
	OrderBy  string // sort clause, e.g. "updated DESC"
}

// IsEmpty reports whether no filter was supplied.
func (p JQLParams) IsEmpty() bool {
	return p == JQLParams{}
}

// BuildJQL assembles a JQL string from the filters, AND-joining each clause
// and appending an ORDER BY when requested.
func BuildJQL(p JQLParams) (string, error) {
	var clauses []string
	if p.Project != "" {
		clauses = append(clauses, `project = `+quote(p.Project))
	}
	if p.Assignee != "" {
		clauses = append(clauses, userClause("assignee", p.Assignee))
	}
	if p.Reporter != "" {
		clauses = append(clauses, userClause("reporter", p.Reporter))
	}
	if p.Status != "" {
		clauses = append(clauses, `status = `+quote(p.Status))
	}
	if p.Type != "" {
		clauses = append(clauses, `issuetype = `+quote(p.Type))
	}
	if p.Label != "" {
		clauses = append(clauses, `labels = `+quote(p.Label))
	}
	if p.Text != "" {
		clauses = append(clauses, `text ~ `+quote(p.Text))
	}
	if len(clauses) == 0 {
		return "", cerrors.New(cerrors.CategoryUsage, "JQL_EMPTY",
			"no search filters were provided").
			WithNextSteps("Pass a raw JQL string, or use --project/--assignee/--status/--type/--label/--text.")
	}
	jql := strings.Join(clauses, " AND ")
	if p.OrderBy != "" {
		jql += " ORDER BY " + p.OrderBy
	}
	return jql, nil
}

// userClause renders a user-field filter, mapping the "me" and "unassigned"
// conveniences onto their JQL forms.
func userClause(field, value string) string {
	switch strings.ToLower(value) {
	case "me":
		return field + " = currentUser()"
	case "unassigned", "empty":
		return field + " is EMPTY"
	}
	return field + ` = ` + quote(value)
}

// quote wraps a value in double quotes, escaping any embedded quotes.
func quote(v string) string {
	return `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
}
