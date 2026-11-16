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
code and the URL of `https://<host>/login/device`; open it, sign in and enter the
code. Personal access tokens (prefix `erp_`) are created in the web UI under
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
| `eg repo clone <repo> [dir] [-- gitflags]` | adds an `upstream` remote for forks |
| `eg repo create [name]` | `--public/--private/--internal`, `-d`, `--clone`, `--source . --push`, `--remote` |
| `eg repo delete [repo] [--yes]` | |
| `eg repo edit [repo]` | `--description`, `--default-branch`, `--visibility` |
| `eg repo rename [new-name]` | updates the matching git remote |
| `eg repo sync` | local sync from the remote (fast-forward or `--force`) |
| `eg browse [number \| path[:line] \| sha]` | `--branch`, `--commit`, `--settings`, `--releases`, `--actions` |

Repository resolution follows gh: `--repo/-R [HOST/]OWNER/REPO`, then `EG_REPO`,
then the git remotes that point to a logged-in host (`upstream`, then `origin`).
Run `eg help environment` for `EG_HOST`, `EG_TOKEN`, `EG_REPO`, `EG_CONFIG_DIR`,
`EG_DEBUG`, `EG_FORCE_TTY`, `EG_PROMPT_DISABLED`, `NO_COLOR`.

Exit codes: 0 success, 1 error, 2 cancelled, 4 authentication required.

## Differences from gh

- There is no default host: `eg` uses `EG_HOST`, else the first logged-in host.
  `EG_TOKEN` applies to every host.
- `eg` never starts a browser or a pager (the only process it runs is `git`):
  `--web`, `eg browse` and the login flow print the URL to open instead.
- `--jq` supports simple paths only (`.name`, `.[].name`, `.items | .[0]`);
  `--template` is not supported.
- Tokens are always stored in `hosts.yml` (no keyring).
- Not supported by the server yet: `repo fork` (no `POST /repos/{owner}/{repo}/forks`),
  remote `repo sync` (no `merge-upstream`), `repo create --template/--gitignore/--license/--add-readme`,
  `repo edit` settings other than description, default branch and visibility, `browse --wiki/--projects`.
- Not implemented yet: `issue`, `pr`, `release`, `run`, `workflow`, `search`, `gist`,
  `status`, `auth switch/refresh`, `repo set-default`, `config`, `alias`, `extension`.

## Development

`make build`, `make vet`, `make fmt-check`, `make lint`. Standard library only.
