package apiclient

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	cerrors "github.com/angelmsger/jira-cli/pkg/errors"
)

// rawProject is the wire shape of a project, common to both flavors.
type rawProject struct {
	ID             string `json:"id"`
	Key            string `json:"key"`
	Name           string `json:"name"`
	ProjectTypeKey string `json:"projectTypeKey"`
	Lead           *struct {
		DisplayName string `json:"displayName"`
	} `json:"lead"`
}

func mapProject(r rawProject, baseURL string) Project {
	p := Project{
		ID:   r.ID,
		Key:  r.Key,
		Name: r.Name,
		Type: r.ProjectTypeKey,
	}
	if r.Lead != nil {
		p.Lead = r.Lead.DisplayName
	}
	if r.Key != "" && baseURL != "" {
		p.URL = baseURL + "/browse/" + r.Key
	}
	return p
}

// ListProjects enumerates projects visible to the caller.
//
//	Cloud: GET /rest/api/3/project/search?startAt&maxResults[&query] (paginated)
//	DC:    GET /rest/api/2/project — returns the full list in one response
//	       (the endpoint is unpaginated), so the result is a single page with
//	       no continuation cursor; Query is applied client-side.
func (c *apiClient) ListProjects(ctx context.Context, opt ProjectListOpts) (ListResult[Project], error) {
	limit := c.limitOf(opt.ListOpts)
	if c.flavor == FlavorCloud {
		q := offsetQuery(opt.Cursor, limit)
		if opt.Query != "" {
			q.Set("query", opt.Query)
		}
		var raw struct {
			Values  []rawProject `json:"values"`
			StartAt int          `json:"startAt"`
			Total   int          `json:"total"`
			IsLast  bool         `json:"isLast"`
		}
		if err := c.getJSON(ctx, c.apiBase()+"/project/search", q, &raw); err != nil {
			return ListResult[Project]{}, err
		}
		res := ListResult[Project]{}
		if !raw.IsLast {
			res.Next = nextOffsetToken(strconv.Itoa(raw.StartAt), limit, len(raw.Values), raw.Total)
		}
		for _, r := range raw.Values {
			res.Items = append(res.Items, mapProject(r, c.baseURL))
		}
		return res, nil
	}

	var raw []rawProject
	if err := c.getJSON(ctx, c.apiBase()+"/project", nil, &raw); err != nil {
		return ListResult[Project]{}, err
	}
	res := ListResult[Project]{}
	query := strings.ToLower(opt.Query)
	for _, r := range raw {
		if query != "" &&
			!strings.Contains(strings.ToLower(r.Name), query) &&
			!strings.Contains(strings.ToLower(r.Key), query) {
			continue
		}
		res.Items = append(res.Items, mapProject(r, c.baseURL))
	}
	return res, nil
}

// GetProject fetches a single project by key.
func (c *apiClient) GetProject(ctx context.Context, key string) (*Project, error) {
	if key == "" {
		return nil, cerrors.New(cerrors.CategoryUsage, "PROJECT_NO_KEY", "a project key is required")
	}
	var raw rawProject
	if err := c.getJSON(ctx, c.apiBase()+"/project/"+url.PathEscape(key), nil, &raw); err != nil {
		return nil, err
	}
	p := mapProject(raw, c.baseURL)
	return &p, nil
}
