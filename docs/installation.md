# Installation & setup guide

This guide covers three things:

1. [Installing the `jira-cli` binary](#1-install-the-cli)
2. [Enabling shell completion](#2-enable-shell-completion)
3. [Installing & updating the companion `jira` Skill](#3-install-the-companion-skill)

---

## 1. Install the CLI

**npm is the recommended way to install** — it downloads the prebuilt binary
for your platform, verifies its checksum, and keeps upgrades a single
`npm update -g` away. The *Other methods* below are alternatives.

### npm (recommended)

```bash
npm install -g @angelmsger/jira-cli
```

Installing downloads the prebuilt binary for your platform from the matching
GitHub Release and verifies its SHA-256 checksum. If your npm setup disables
install scripts (`--ignore-scripts`, some pnpm setups), the binary is fetched on
first run instead.

### Other methods

Prefer not to use npm? Any of these also work.

#### go install

```bash
go install github.com/angelmsger/jira-cli/cmd/jira-cli@latest
```

Installs into `go env GOBIN` (or `$GOPATH/bin`). Requires Go 1.24+.

#### Prebuilt binary

Download the binary for your platform from the
[Releases page](https://github.com/angelmsger/jira-cli/releases), verify
it against `checksums.txt`, then put it on your `PATH`.

On macOS/Linux:

```bash
chmod +x jira-cli-* && mv jira-cli-* /usr/local/bin/jira-cli
```

On Windows PowerShell, download `jira-cli-windows-amd64.exe` (or
`windows-arm64.exe`) together with `checksums.txt`, then:

```powershell
$asset = "jira-cli-windows-amd64.exe"
$checksumLine = Get-Content .\checksums.txt | Where-Object { $_ -match "\s+$([regex]::Escape($asset))$" } | Select-Object -First 1
if (-not $checksumLine) { throw "No checksum found for $asset" }
$expected = ($checksumLine -split '\s+')[0].ToLowerInvariant()
$actual = (Get-FileHash ".\$asset" -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actual -ne $expected) { throw "SHA-256 mismatch for $asset" }
$binDir = Join-Path $HOME "bin"
New-Item -ItemType Directory -Force $binDir | Out-Null
Move-Item ".\$asset" (Join-Path $binDir "jira-cli.exe")
[Environment]::SetEnvironmentVariable("Path", ([Environment]::GetEnvironmentVariable("Path", "User") + ";$binDir"), "User")
```

Open a new PowerShell window after changing `PATH`.

#### From source

```bash
git clone https://github.com/angelmsger/jira-cli.git && cd conflunce-cli
make install        # builds and installs into `go env GOBIN` or $GOPATH/bin
```

`make install` prints the install path. Make sure that directory is on your
`PATH`:

```bash
echo 'export PATH="$(go env GOPATH)/bin:$PATH"' >> ~/.zshrc   # or ~/.bashrc
```

Other build targets: `make build` (to `./bin/`), `make cross` (every platform
into `./dist/`).

### First-time configuration

```bash
jira-cli config init --pretty   # interactive TUI: server URL, flavor, credentials
jira-cli doctor                 # verify configuration and connectivity
```

For headless setup in PowerShell, environment variables use `$env:` syntax:

```powershell
$env:JIRA_SERVER = "https://example.atlassian.net"
$env:JIRA_USERNAME = "alice@example.com"
$env:JIRA_API_TOKEN = "<api-token>"
jira-cli doctor
```

The `--pretty` flag opts into a `huh`-based TUI with arrow-key selection,
masked password input, and Shift-Tab back-navigation. Without it,
`config init` runs as a plain line-by-line wizard — keep that form for
scripted setup, dotfiles bootstrap, and non-TTY environments where a TUI
cannot render.

When the server URL is on `*.atlassian.net` (Cloud), the wizard now
defaults the auth scheme to **basic** and asks for your Atlassian email
plus an API token from
[id.atlassian.com](https://id.atlassian.com/manage-profile/security/api-tokens).
Cloud's REST API only accepts those tokens via HTTP Basic — `pat`
(Bearer) is Data Center only and 403s on Cloud, so the wizard saves you
from picking the wrong scheme.

---

## 2. Enable shell completion

`jira-cli` completes subcommands, enum flag values (`--format`, `--flavor`,
...) and **live project keys** for `project get <key>`.

The CLI ships the completion *logic*, but every shell needs the completion
*script* loaded once. Pick your shell below.

### bash

```bash
# try it in the current shell
source <(jira-cli completion bash)

# make it permanent (Linux)
jira-cli completion bash | sudo tee /etc/bash_completion.d/jira-cli >/dev/null

# make it permanent (macOS, Homebrew bash-completion)
jira-cli completion bash > "$(brew --prefix)/etc/bash_completion.d/jira-cli"
```

bash needs the `bash-completion` package installed and sourced from your
`~/.bashrc`.

### zsh

```bash
# ensure compinit runs — add this to ~/.zshrc if it is not there already:
#   autoload -Uz compinit && compinit

# install the completion into a directory on $fpath
jira-cli completion zsh > "${fpath[1]}/_jira-cli"
```

Open a new shell afterwards. If completions still do not appear, run
`rm -f ~/.zcompdump*` and start a new shell.

### fish

```bash
jira-cli completion fish > ~/.config/fish/completions/jira-cli.fish
```

### PowerShell

```powershell
# current session
jira-cli completion powershell | Out-String | Invoke-Expression

# persistent — append to your profile
jira-cli completion powershell >> $PROFILE
```

Run `jira-cli completion --help` for the authoritative per-shell notes.

### Verifying

After loading the script, type `jira-cli --format ` and press `<TAB>` — you
should see `json table ndjson`. For live project-key completion,
`jira-cli project get <TAB>` queries the configured server (best-effort; it
shows nothing if the CLI is not configured yet).

---

## 3. Install the companion Skill

The `jira` Skill teaches a coding agent — **Claude Code**, **Codex**, and **Grok Build** —
how to drive this CLI. It is **embedded in the `jira-cli` binary**, so
whichever way you installed the CLI — npm, `go install`, a prebuilt binary —
you already have a version-matched copy of the Skill.

### Recommended: `jira-cli skill install`

With no flags, `skill install` **probes for installed agents** and installs the
Skill into every one it finds:

```bash
jira-cli skill install              # auto-detect; install for each agent found
jira-cli skill install --agent codex          # only Codex
jira-cli skill install --agent claude-code,codex,grok
jira-cli skill install --project    # project dirs instead of $HOME
jira-cli skill install --dir <path> # explicit base -> <path>/jira

jira-cli skill path                 # show every agent's location + status
jira-cli skill show                 # print SKILL.md to stdout
```

Install locations per agent:

| Agent | Global (default) | Project (`--project`) |
|-------|------------------|-----------------------|
| Claude Code | `~/.claude/skills/jira` | `./.claude/skills/jira` |
| Codex | `~/.codex/skills/jira` | `./.agents/skills/jira` |
| Grok Build | `~/.grok/skills/jira` | `./.grok/skills/jira` |

Auto-detection looks for `~/.claude` / `~/.codex` / `~/.grok` (global) or `./.claude` /
`./.agents` / `./AGENTS.md` / `./.grok` (project). If nothing is detected, pass `--agent`
or `--dir` explicitly.

Because the Skill ships inside the binary, **updating is automatic**: upgrade
the CLI (`npm update -g @angelmsger/jira-cli`, `go install ...@latest`,
etc.) and re-run `jira-cli skill install` — the deployed Skill always
matches the CLI version.

### Alternative: the `skills` CLI

If you manage agent skills with the [`skills` tool](https://github.com/vercel-labs/skills)
(`npx skills`), you can install the Skill straight from the repository:

```bash
npx skills add angelmsger/jira-cli --skill jira       # this project
npx skills add angelmsger/jira-cli --skill jira -g    # all projects
npx skills add ./skills/jira                                # local checkout
npx skills update jira                                      # refresh later
```

Useful flags: `-a claude-code` targets a specific agent, `-y` runs
non-interactively, `--list` previews a repo's skills.

> **Maintainers:** bump `version:` in `skills/jira/SKILL.md` on every
> change to the Skill or its `references/`, so both `jira-cli skill show`
> and `npx skills update` report the new version.

### Removing the Skill

```bash
jira-cli skill uninstall          # auto-detect; remove from each agent found
jira-cli skill uninstall --agent codex
jira-cli skill uninstall --dir <path>
npx skills remove jira            # if installed via the skills CLI
```

`skill uninstall` takes the same `--agent` / `--project` / `--dir` flags as
`skill install`; removing a Skill that is not installed is a no-op.
