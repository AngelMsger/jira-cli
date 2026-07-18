// Command mockserver is a minimal in-memory Jira Data Center REST API (v2),
// used by scripts/e2e.sh to exercise jira-cli end-to-end without a real
// server. It prints its base URL on the first line of stdout, then serves.
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
)

func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, "mockserver: listen failed:", err)
		os.Exit(1)
	}
	fmt.Printf("http://%s\n", ln.Addr().String())
	os.Stdout.Sync()

	if err := http.Serve(ln, routes()); err != nil {
		fmt.Fprintln(os.Stderr, "mockserver:", err)
		os.Exit(1)
	}
}

func routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /rest/api/2/serverInfo", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"baseUrl": "http://" + r.Host, "version": "9.12.0",
			"deploymentType": "Server", "serverTitle": "Mock Jira",
		})
	})

	mux.HandleFunc("GET /rest/api/2/myself", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, user("alice", "Alice Example"))
	})

	mux.HandleFunc("GET /rest/api/2/project", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, []any{project("ENG", "Engineering"), project("OPS", "Operations")})
	})
	mux.HandleFunc("GET /rest/api/2/project/{key}", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		if key == "NOPE" {
			jiraError(w, http.StatusNotFound, "No project could be found with key 'NOPE'.")
			return
		}
		writeJSON(w, project(key, "Engineering"))
	})

	// Metadata discovery (DC dialect: full-list arrays, createmeta `values`).
	mux.HandleFunc("GET /rest/api/2/project/{key}/components", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, []any{
			map[string]any{"id": "10000", "name": "PaaS", "description": "Platform services",
				"lead": map[string]any{"displayName": "Alice Example"}},
			map[string]any{"id": "10001", "name": "IaaS"},
		})
	})
	mux.HandleFunc("GET /rest/api/2/project/{key}/versions", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, []any{
			map[string]any{"id": "20000", "name": "1.0.0", "released": true, "archived": false,
				"releaseDate": "2026-01-15"},
			map[string]any{"id": "20001", "name": "1.1.0", "released": false, "archived": false},
		})
	})
	mux.HandleFunc("GET /rest/api/2/project/{key}/statuses", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, []any{
			map[string]any{"id": "1", "name": "Bug", "statuses": []any{
				status("1", "Open", "new"), status("3", "In Progress", "indeterminate"),
				status("6", "Closed", "done"),
			}},
			map[string]any{"id": "3", "name": "Task", "statuses": []any{
				status("1", "Open", "new"), status("6", "Closed", "done"),
			}},
		})
	})
	mux.HandleFunc("GET /rest/api/2/issue/createmeta/{key}/issuetypes", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"values": []any{
			map[string]any{"id": "1", "name": "Bug", "subtask": false},
			map[string]any{"id": "3", "name": "Task", "subtask": false},
		}, "isLast": true})
	})
	mux.HandleFunc("GET /rest/api/2/issue/createmeta/{key}/issuetypes/{typeId}", func(w http.ResponseWriter, r *http.Request) {
		fields := []any{
			map[string]any{
				"fieldId": "summary", "name": "Summary", "required": true,
				"schema":     map[string]any{"type": "string", "system": "summary"},
				"operations": []any{"set"},
			},
			map[string]any{
				"fieldId": "components", "name": "Component/s", "required": false,
				"schema":     map[string]any{"type": "array", "items": "component", "system": "components"},
				"operations": []any{"add", "set", "remove"},
				"allowedValues": []any{
					map[string]any{"id": "10000", "name": "PaaS"},
					map[string]any{"id": "10001", "name": "IaaS"},
				},
			},
		}
		// Severity exists on Bug (type 1) only, so `field options` exercises
		// the per-issue-type annotation when scanning all types.
		if r.PathValue("typeId") == "1" {
			fields = append(fields, map[string]any{
				"fieldId": "customfield_10010", "name": "Severity", "required": false,
				"schema": map[string]any{"type": "option",
					"custom": "com.atlassian.jira.plugin.system.customfieldtypes:select"},
				"operations": []any{"set"},
				"allowedValues": []any{
					map[string]any{"id": 1, "value": "Critical"},
					map[string]any{"id": 2, "value": "Minor"},
				},
			})
		}
		writeJSON(w, map[string]any{"values": fields, "isLast": true})
	})
	mux.HandleFunc("GET /rest/api/2/priority", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, []any{
			map[string]any{"id": "2", "name": "High"},
			map[string]any{"id": "3", "name": "Medium"},
		})
	})

	// GET and POST /search share one handler: the CLI uses GET on DC, but a
	// POST form exists too and behaves identically here.
	search := func(w http.ResponseWriter, r *http.Request) {
		startAt, _ := strconv.Atoi(r.URL.Query().Get("startAt"))
		all := []any{
			issue("ENG-1", "Welcome to the mock backlog"),
			issue("ENG-2", "Second issue"),
			issue("ENG-3", "Third issue"),
		}
		end := startAt + 2 // page size 2, so --all exercises pagination
		if end > len(all) {
			end = len(all)
		}
		page := []any{}
		if startAt < len(all) {
			page = all[startAt:end]
		}
		writeJSON(w, map[string]any{
			"issues": page, "startAt": startAt,
			"maxResults": 2, "total": len(all),
		})
	}
	mux.HandleFunc("GET /rest/api/2/search", search)
	mux.HandleFunc("POST /rest/api/2/search", search)

	mux.HandleFunc("GET /rest/api/2/issue/{key}", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		if key == "ENG-404" {
			jiraError(w, http.StatusNotFound, "Issue does not exist or you do not have permission to see it.")
			return
		}
		writeJSON(w, issue(key, "Welcome to the mock backlog"))
	})
	mux.HandleFunc("POST /rest/api/2/issue", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Fields struct {
				Summary string `json:"summary"`
			} `json:"fields"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Fields.Summary == "" {
			jiraError(w, http.StatusBadRequest, "You must specify a summary of the issue.")
			return
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]any{"id": "10100", "key": "ENG-100"})
	})
	mux.HandleFunc("PUT /rest/api/2/issue/{key}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("PUT /rest/api/2/issue/{key}/assignee", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /rest/api/2/issue/{key}/transitions", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"transitions": []any{
			transition("21", "Start Progress", "In Progress", "indeterminate"),
			transition("31", "Done", "Done", "done"),
		}})
	})
	mux.HandleFunc("POST /rest/api/2/issue/{key}/transitions", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Transition struct {
				ID string `json:"id"`
			} `json:"transition"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Transition.ID != "21" && body.Transition.ID != "31" {
			jiraError(w, http.StatusBadRequest, "The transition is not valid for this issue.")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /rest/api/2/issue/{key}/comment", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"comments": []any{comment("10001", "First comment")},
			"startAt":  0, "maxResults": 25, "total": 1,
		})
	})
	mux.HandleFunc("POST /rest/api/2/issue/{key}/comment", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Body string `json:"body"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, comment("10099", body.Body))
	})
	mux.HandleFunc("PUT /rest/api/2/issue/{key}/comment/{id}", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Body string `json:"body"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		writeJSON(w, comment(r.PathValue("id"), body.Body))
	})
	mux.HandleFunc("DELETE /rest/api/2/issue/{key}/comment/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") == "404" {
			jiraError(w, http.StatusNotFound, "Comment does not exist.")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GitHub-style latest-release endpoint for the update notifier
	// (JIRA_RELEASE_API points here in e2e).
	mux.HandleFunc("GET /releases/latest", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"tag_name": "v9.9.9"})
	})

	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// jiraError writes Jira's error envelope: {"errorMessages":[...],"errors":{}}.
func jiraError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"errorMessages": []string{msg}, "errors": map[string]string{},
	})
}

func user(name, display string) map[string]any {
	return map[string]any{
		"name": name, "key": name, "displayName": display,
		"emailAddress": name + "@example.com", "active": true,
	}
}

func project(key, name string) map[string]any {
	return map[string]any{
		"id": "10000", "key": key, "name": name,
		"projectTypeKey": "software",
		"lead":           map[string]any{"displayName": "Alice Example"},
	}
}

func issue(key, summary string) map[string]any {
	return map[string]any{
		"id": "10001", "key": key,
		"fields": map[string]any{
			"summary":     summary,
			"description": "A plain text description.",
			"status": map[string]any{
				"id": "3", "name": "In Progress",
				"statusCategory": map[string]any{"key": "indeterminate"},
			},
			"issuetype":   map[string]any{"name": "Task"},
			"priority":    map[string]any{"name": "Medium"},
			"assignee":    user("alice", "Alice Example"),
			"reporter":    user("bob", "Bob Example"),
			"labels":      []string{"mock"},
			"components":  []any{map[string]any{"id": "10000", "name": "PaaS"}},
			"fixVersions": []any{map[string]any{"id": "20001", "name": "1.1.0"}},
			"project":     map[string]any{"key": "ENG"},
			"created":     "2026-01-01T00:00:00.000+0000",
			"updated":     "2026-01-02T00:00:00.000+0000",
		},
	}
}

func comment(id, body string) map[string]any {
	return map[string]any{
		"id": id, "author": user("alice", "Alice Example"), "body": body,
		"created": "2026-01-01T00:00:00.000+0000",
		"updated": "2026-01-01T00:00:00.000+0000",
	}
}

func status(id, name, category string) map[string]any {
	return map[string]any{
		"id": id, "name": name,
		"statusCategory": map[string]any{"key": category},
	}
}

func transition(id, name, toName, category string) map[string]any {
	return map[string]any{
		"id": id, "name": name,
		"to": map[string]any{
			"id": "3", "name": toName,
			"statusCategory": map[string]any{"key": category},
		},
	}
}
