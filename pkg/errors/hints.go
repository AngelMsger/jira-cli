package errors

// defaultGuidance returns the default hint and next-step commands for a
// category. Callers may override these via WithHint / WithNextSteps when more
// specific guidance is available.
func defaultGuidance(cat Category) (hint string, steps []string) {
	switch cat {
	case CategoryUsage:
		return "The command was invoked incorrectly. Check flags and arguments.",
			[]string{"jira-cli <command> --help"}
	case CategoryConfig:
		return "No usable configuration was found or it is invalid.",
			[]string{"jira-cli config init", "jira-cli config show --explain"}
	case CategoryAuth:
		return "The server rejected the credentials. The token may be expired or wrong.",
			[]string{"jira-cli auth status", "jira-cli config init"}
	case CategoryPermission:
		return "The credentials are valid but lack permission for this resource.",
			[]string{"Verify the account can access the issue or project in a browser."}
	case CategoryNotFound:
		return "The requested issue, project or comment does not exist.",
			[]string{"jira-cli issue search --text \"<keywords>\" --limit 25", "Double-check the key, ID or URL."}
	case CategoryConflict:
		return "The resource changed since it was last read (version conflict).",
			[]string{"Re-fetch the resource to get its current version, then retry."}
	case CategoryRateLimit:
		return "The server is rate limiting requests.",
			[]string{"Retry reads after a short wait; verify a write's outcome before repeating it.", "Prefer a scoped query and bounded pages over --all."}
	case CategoryNetwork:
		return "The server could not be reached (DNS, TLS or timeout).",
			[]string{"jira-cli doctor", "Check --base-url and network connectivity."}
	case CategoryServer:
		return "The Jira server returned an internal error.",
			[]string{"Retry reads later; verify a write's outcome before repeating it.", "jira-cli doctor"}
	case CategoryParse:
		return "A response could not be parsed or rendered.",
			[]string{"Inspect the error code and verify remote state with a read before repeating a write."}
	default:
		return "An unexpected internal error occurred.",
			[]string{"Retry with --verbose for details."}
	}
}
