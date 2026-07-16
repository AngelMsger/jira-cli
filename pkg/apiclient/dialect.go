package apiclient

import (
	"net/url"
	"strconv"
)

// dialect.go centralises the per-flavor REST differences. Cloud targets REST
// API v3 (ADF bodies, token-paginated JQL search); Data Center / Server
// targets REST API v2 (plain-text bodies, startAt pagination). Most relative
// paths are identical across flavors — the divergences are the version
// prefix, the search endpoint, body representation (see adf.go), and user
// identifier semantics (see users.go).

// apiBase returns the REST base path for the flavor.
//   - Cloud:       /rest/api/3
//   - Data Center: /rest/api/2
func (c *apiClient) apiBase() string {
	if c.flavor == FlavorCloud {
		return "/rest/api/3"
	}
	return "/rest/api/2"
}

// offsetQuery builds startAt/maxResults query parameters for offset
// pagination. The cursor, when present, carries the numeric start index.
func offsetQuery(cursor string, limit int) url.Values {
	q := url.Values{}
	start := 0
	if cursor != "" {
		if n, err := strconv.Atoi(cursor); err == nil {
			start = n
		}
	}
	q.Set("startAt", strconv.Itoa(start))
	q.Set("maxResults", strconv.Itoa(limit))
	return q
}

// nextOffsetToken returns the cursor for the following offset page, or "" when
// the current page was the last one. total < 0 means the response did not
// report a total, in which case a full page implies there may be more.
func nextOffsetToken(cursor string, limit, size, total int) string {
	if limit <= 0 || size < limit {
		return ""
	}
	start := 0
	if cursor != "" {
		if n, err := strconv.Atoi(cursor); err == nil {
			start = n
		}
	}
	next := start + size
	if total >= 0 && next >= total {
		return ""
	}
	return strconv.Itoa(next)
}
