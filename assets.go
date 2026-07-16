// Package jiracli is the module root. It exists only to embed packaged
// assets — the companion `jira` Skill — into the CLI binary, so that
// `jira-cli skill install` can deploy a version-matched copy regardless
// of how the binary itself was installed (npm, go install, prebuilt, source).
package jiracli

import "embed"

// SkillFS holds the companion Skill, rooted at "skills/jira".
//
//go:embed all:skills/jira
var SkillFS embed.FS

// SkillRoot is the path within SkillFS at which the Skill is rooted.
const SkillRoot = "skills/jira"
