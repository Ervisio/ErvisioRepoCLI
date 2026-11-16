# ErvisioRepoCLI

`eg` is the command-line client for [ErvisioRepo](https://github.com/Ervisio/ErvisioRepo).
It works like GitHub's `gh`: same command tree, flags, output formats and exit
codes, talking to the GitHub-compatible API of an ErvisioRepo server
(`/api/v3` and `/api/graphql`). Environment variables use the `EG_` prefix.

[Italiano](README.it.md)

## Install

```sh
go install github.com/Ervisio/ErvisioRepoCLI/cmd/eg@latest
```

Requires Go 1.26 and `git` in `PATH`. From a checkout: `make build` (writes `./eg`)
or `make install`.

## Log in

```sh
eg auth login                         # asks for the server, then browser or token login
eg auth login --hostname code.example.com --web
eg auth login --hostname http://localhost:8080   # plain-http local server
eg auth login --hostname code.example.com --with-token < token.txt
eg auth setup-git                     # use eg as git credential helper
eg auth status
```

The browser login uses the server's OAuth device flow: `eg` prints a one-time
code and, after you press Enter, opens `https://<host>/login/device` in the
browser; sign in and enter the code. Personal access tokens (prefix `erp_`) are created in the web UI under
Settings → Tokens (`https://<host>/settings?section=tokens`) with at least the
`repo:write` and `user:read` scopes.

Credentials live in `~/.config/eg/hosts.yml` (mode 0600), settings in
`~/.config/eg/config.yml`, in the same shape as gh.

## Commands

| Command | Notes |
|---|---|
| `eg auth login / logout / status / token / setup-git` | `git-credential` is the hidden helper used by git |
| `eg api <endpoint>` | `-X`, `-f`, `-F`, `-H`, `--paginate`, `--slurp`, `--input`, `-i`, `--silent`, `--hostname`, `--jq`; `{owner}`, `{repo}`, `{branch}` placeholders; `graphql` |
| `eg repo list [owner]` | `-L`, `--visibility`, `--fork`, `--source`, `--archived`, `--no-archived`, `-l`, `--topic`, `--json` |
| `eg repo view [repo]` | `--web`, `--branch`, `--json` |
| `eg repo clone <repo> [dir] [-- gitflags]` | adds an `upstream` remote for forks (`-u`, `--no-upstream`) |
| `eg repo create [name]` | `--public/--private/--internal`, `-d`, `--clone`, `--source . --push`, `--remote`, `--add-readme`, `-g/--gitignore`, `-l/--license` |
| `eg repo fork [repo] [-- gitflags]` | `--clone`, `--remote`, `--remote-name`, `--org`, `--fork-name`; renames `origin` to `upstream` like gh |
| `eg repo gitignore list / view`, `eg repo license list / view` | templates of the server; `license view --web` |
| `eg repo delete [repo] [--yes]` | |
| `eg repo edit [repo]` | `--description`, `--default-branch`, `--visibility` |
| `eg repo rename [new-name]` | updates the matching git remote |
| `eg repo sync [fork]` | local sync from the remote (fast-forward or `--force`); with a fork argument, `merge-upstream` (fast-forward only) |
| `eg browse [number \| path[:line] \| sha]` | `--branch`, `--commit`, `--settings`, `--releases`, `--actions`, `--no-browser` |
| `eg issue list` | `-s`, `-l`, `-a`, `-A`, `--mention`, `-m`, `-S/--search`, `-L`, `--json`, `--web` |
| `eg issue view <n>` | `--comments`, `--json`, `--web` |
| `eg issue create` | `-t`, `-b`, `-F`, `-e`, `-a`, `-l`, `-m`, `--web`; prompts and editor on a terminal |
| `eg issue edit <n>...` | `-t`, `-b`, `-F`, `--add/remove-label`, `--add/remove-assignee`, `-m`, `--remove-milestone` |
| `eg issue close / reopen <n>` | `-c`, `-r completed\|not planned\|duplicate`, `--duplicate-of` |
| `eg issue comment <n>` | `-b`, `-F`, `-e`, `--web`, `--edit-last`, `--delete-last`, `--create-if-none` |
| `eg issue status` | assigned, mentioning and opened by you |
| `eg pr list` | `-s open\|closed\|merged\|all`, `-B`, `-H`, `-l`, `-a`, `-A`, `-d`, `-S`, `-L`, `--json`, `--web` |
| `eg pr view / diff [n\|url\|branch]` | `--comments`, `--json`, `--web`; diff `--patch`, `--name-only`, `--color` |
| `eg pr create` | `-t`, `-b`, `-F`, `-B`, `-H`, `-d`, `-f`, `--fill-first`, `-e`, `-a`, `-l`, `-m`, `-r`, `--web`, `--dry-run`, `--no-maintainer-edit`; pushes the branch (asks first) |
| `eg pr checkout <n>` | `-b`, `-f`, `--detach`, `--recurse-submodules`; forks through `refs/pull/<n>/head` |
| `eg pr merge` | `-m/-s/-r`, `-t`, `-b`, `-F`, `-d`, `--auto`, `--disable-auto`, `--match-head-commit` |
| `eg pr close / reopen / ready / comment / edit / review` | as gh; `ready --undo`, `close -d`, `edit -B --add/remove-reviewer` |
| `eg pr status / checks` | checks: statuses and check runs, `--watch`, `--fail-fast`, `-i`, `--json`; exit 8 when pending |
| `eg release list / view / create / edit / delete` | `--notes`, `-F`, `-t`, `--target`, `-d`, `-p`, `--verify-tag`, `--notes-from-tag`, `--cleanup-tag` |
| `eg release upload / download / delete-asset` | `--clobber`; download `-p`, `-D`, `-O`, `-A zip\|tar.gz`, `--skip-existing` |
| `eg label list / create / edit / delete` | `-S`, `--sort`, `--order`, `-c`, `-d`, `-f`, `-n` |
| `eg run list` | `-L`, `-w`, `-b`, `-u`, `-e`, `-s`, `-c`, `--created`, `--json` |
| `eg run view [id]` | `--log`, `--log-failed`, `-j`, `-v`, `-a`, `--exit-status`, `--web`, `--json`; imported GitHub runs show their GitHub link |
| `eg run watch / rerun / cancel [id]` | watch `-i`, `--exit-status`, `--compact`; rerun of whole runs only |
| `eg run download [id]` | `-n`, `-p`, `-D`; artifacts extracted from their zip |
| `eg workflow list / view / run` | view `-y`, `-r`, `--web`; run `-f`, `-F` (`@file`), `--ref`, `--json` |
| `eg secret list / set / delete` | repository scope; `set -b`, `-f .env`, `--no-store`; values sealed with the repository key |
| `eg variable list / get / set / delete` | repository scope; `set -b`, `-f .env`; `--json` |
| `eg search repos / issues / prs / code` | gh's qualifier flags; issues, prs and code search one repository (`--repo`, `repo:` or the current clone) |
| `eg alias set / list / delete` | `$1` placeholders, `--clobber`, `-` reads the expansion from stdin; no shell aliases |
| `eg config get / set / list` | `git_protocol`, `editor`, `prompt`, `pager`, `browser`; `-h` per host |
| `eg completion -s bash\|zsh\|fish\|powershell` | completes commands, flags and aliases from the command tree |

Repository resolution follows gh: `--repo/-R [HOST/]OWNER/REPO`, then `EG_REPO`,
then the git remotes that point to a logged-in host (`upstream`, then `origin`).
Run `eg help environment` for `EG_HOST`, `EG_TOKEN`, `EG_REPO`, `EG_CONFIG_DIR`,
`EG_BROWSER`, `EG_PAGER`, `EG_EDITOR`, `EG_DEBUG`, `EG_FORCE_TTY`, `EG_PROMPT_DISABLED`,
`NO_COLOR`, `CLICOLOR_FORCE`.

Like gh, `eg` opens the browser (`EG_BROWSER`, config `browser`, `BROWSER`, else
`open`/`xdg-open`/`rundll32`), pages long output on a terminal (`EG_PAGER`, config
`pager`, `PAGER`, else `less -R` with `LESS=FRX`) and opens an editor for bodies
(`EG_EDITOR`, config `editor`, `VISUAL`, `EDITOR`, else `nano`). The values are split
into words like a shell would, but never run through a shell.

Exit codes: 0 success, 1 error, 2 cancelled, 4 authentication required, 8 checks pending.

## Differences from gh

- There is no default host: `eg` uses `EG_HOST`, else the first logged-in host.
  `EG_TOKEN` applies to every host.
- `--jq` supports simple paths only (`.name`, `.[].name`, `.items | .[0]`);
  `--template` is not supported.
- Tokens are always stored in `hosts.yml` (no keyring).
- Not supported by the server yet: `repo create --template`, `repo edit` settings other than
  description, default branch and visibility, `browse --wiki/--projects`, `issue delete` (no `deleteIssue`
  mutation), `issue lock/unlock` (no `PUT/DELETE .../issues/{n}/lock`), `release create
  --generate-notes/--notes-start-tag` (no `POST .../releases/generate-notes`), `release --latest`
  (`make_latest` is ignored), `pr merge --admin/--author-email`, `pr checks --required`, team reviewers
  and projects, `workflow enable/disable`, `run delete`, `run rerun --failed/--job/--debug`, org,
  environment and user secrets and variables (`--org`, `--env`, `--user`), `search commits`, remote
  `repo sync --force`. `repo fork --default-branch-only` is accepted but every branch is copied.
- `issue list`/`pr list` print "Showing N of M" from the REST pages (up to 1000 items counted);
  `--search`, `status` and `eg search issues/prs/code` need one repository (`repo:` qualifier), which
  `eg` takes from `--repo` or the current clone.
- Runs imported from GitHub (GitHub sync) show their summary and GitHub link; their jobs, logs and
  artifacts stay on GitHub.
- Aliases expand in-process: shell aliases (`!`, `--shell`) are refused because `eg` never runs a shell.
- Not implemented yet: `issue develop/pin/unpin/transfer`, `label clone`, `gist`, `status` (the
  server cannot search across repositories), `auth switch/refresh`, `repo set-default`, `extension`,
  `cache`, `alias import`, interactive `workflow run` input prompts.

## Development

`make build`, `make vet`, `make fmt-check`, `make lint`. Standard library only, plus
`golang.org/x/crypto` for the libsodium sealed box of `eg secret set`.
