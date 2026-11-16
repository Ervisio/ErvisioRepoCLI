# Changelog

All notable changes to this project are documented in this file.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- Phase 3: `eg run` (`list`, `view` with `--log`/`--log-failed` from the log zip, `--job`,
  `--verbose`, `--attempt`, `--exit-status`, `--web`, `--json`; `watch`, `rerun`, `cancel`,
  `download` of artifacts by `-n`/`-p` into `-D`). Runs imported from GitHub show their summary
  and GitHub link. `run delete` and `rerun --failed/--job` report that the server lacks them.
- `eg workflow` (`list`, `view` with `--yaml`/`--ref`/`--web`, `run` with `-f`/`-F`/`--json`
  inputs and `--ref`); `enable`/`disable` report that workflows cannot be disabled.
- `eg secret` (`list`, `set`, `delete`; values sealed with the repository public key, dotenv
  files, `--no-store`) and `eg variable` (`list`, `get`, `set`, `delete`), repository scope only.
- `eg search repos`, `issues`, `prs` and `code` with gh's qualifier flags; issue and code
  searches take the repository from `--repo`, a `repo:` qualifier or the current clone.
- `eg repo fork` (`--clone`, `--remote`, `--remote-name`, `--org`, `--fork-name`), remote
  `eg repo sync <fork>` through `merge-upstream`, `eg repo create --add-readme --gitignore
  --license` (also asked interactively), `eg repo gitignore list|view`, `eg repo license list|view`.
- `eg alias set|list|delete` with `$N` placeholders and alias expansion; `eg config
  get|set|list` (`git_protocol`, `editor`, `prompt`, `pager`, `browser`, per host with `-h`);
  `prompt disabled` turns prompts off.
- `eg completion -s bash|zsh|fish|powershell`, backed by the hidden `eg __complete`.

- Phase 2a: `eg issue` (`list`, `view`, `create`, `edit`, `close`, `reopen`, `comment`,
  `status`; `delete`, `lock` and `unlock` report the missing server endpoints).
- `eg pr` (`list`, `view`, `create`, `checkout`, `diff`, `merge`, `close`, `reopen`,
  `comment`, `edit`, `review`, `ready`, `status`, `checks`): checkout of forks through
  `refs/pull/<n>/head`, `create` pushes the branch after asking, `merge --auto` through
  GraphQL, `checks` from commit statuses and check runs (exit code 8 when pending).
- `eg release` (`list`, `view`, `create`, `edit`, `delete`, `upload`, `download`,
  `delete-asset`) with uploads through `/api/uploads` and source archive downloads.
- `eg label` (`list`, `create`, `edit`, `delete`).
- Browser, pager and editor like gh: `EG_BROWSER`/`BROWSER`, `EG_PAGER`/`PAGER`
  (default `less -R` with `LESS=FRX`), `EG_EDITOR`/`VISUAL`/`EDITOR`, and the `browser`,
  `pager` and `editor` keys of `config.yml`; commands are split into argv, never run
  through a shell. `--web`, `eg browse` and the device login open the browser.
- Relative times, state colors and true-color label colors in tables.

### Changed

- `go.mod` now requires `golang.org/x/crypto` (`nacl/box`) for the libsodium sealed box of
  `eg secret set`; it is the only dependency.
- `eg browse` opens the browser; `--no-browser` prints the URL.

- `eg`, a gh-compatible CLI for ErvisioRepo, built with the Go standard library only.
- `eg auth login` (OAuth device flow by default, `--with-token`, `--git-protocol`, plain-http
  hosts), `logout`, `status`, `token`, `setup-git` and the `git-credential` helper.
- `eg api` with fields, headers, `--input`, `--paginate` (REST Link header and GraphQL
  `$endCursor`), `--slurp`, `-i`, `--silent` and a simple-path `--jq`.
- `eg repo list`, `view`, `clone`, `create`, `delete`, `edit`, `rename`, local `sync`;
  `repo fork` reports that the server does not support it yet.
- `eg browse` (prints the URL to open).
- gh-style help, `--json` field lists, TTY tables and tab-separated output, exit codes
  0/1/2/4, `EG_HOST`, `EG_TOKEN`, `EG_REPO`, `EG_CONFIG_DIR`, `EG_DEBUG`, `NO_COLOR`.
- `hosts.yml` and `config.yml` in gh's layout, read and written by a minimal YAML parser.
