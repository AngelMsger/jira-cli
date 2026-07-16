// Command jira-cli lets Coding Agents use a Jira instance as an
// external knowledge base: read pages, search via CQL, and manage comments.
package main

import (
	"os"

	"github.com/angelmsger/jira-cli/internal/app"
)

func main() {
	os.Exit(app.Execute())
}
