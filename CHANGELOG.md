# Changelog

All notable changes to this project are documented in this file.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

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
