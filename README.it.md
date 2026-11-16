# ErvisioRepoCLI

`eg` è il client da riga di comando di [ErvisioRepo](https://github.com/Ervisio/ErvisioRepo).
Funziona come `gh` di GitHub: stessi comandi, flag, formati di output e codici di
uscita, e parla con l'API compatibile GitHub di un server ErvisioRepo (`/api/v3` e
`/api/graphql`). Le variabili d'ambiente usano il prefisso `EG_`.

[English](README.md)

## Installazione

```sh
go install github.com/Ervisio/ErvisioRepoCLI/cmd/eg@latest
```

Servono Go 1.26 e `git` nel `PATH`. Da un checkout: `make build` (crea `./eg`) o
`make install`.

## Accesso

```sh
eg auth login                         # chiede il server, poi login via browser o token
eg auth login --hostname code.example.com --web
eg auth login --hostname http://localhost:8080   # server locale in http
eg auth login --hostname code.example.com --with-token < token.txt
eg auth setup-git                     # eg come credential helper di git
eg auth status
```

Il login via browser usa l'OAuth device flow del server: `eg` stampa un codice
monouso e, dopo Invio, apre `https://<host>/login/device` nel browser; accedi e
inserisci il codice. I personal access token (prefisso `erp_`) si creano dall'interfaccia web
in Impostazioni → Token (`https://<host>/settings?section=tokens`) con almeno gli
scope `repo:write` e `user:read`.

Le credenziali stanno in `~/.config/eg/hosts.yml` (permessi 0600), le impostazioni
in `~/.config/eg/config.yml`, con la stessa struttura di gh.

## Comandi

| Comando | Note |
|---|---|
| `eg auth login / logout / status / token / setup-git` | `git-credential` è l'helper nascosto usato da git |
| `eg api <endpoint>` | `-X`, `-f`, `-F`, `-H`, `--paginate`, `--slurp`, `--input`, `-i`, `--silent`, `--hostname`, `--jq`; segnaposto `{owner}`, `{repo}`, `{branch}`; `graphql` |
| `eg repo list [owner]` | `-L`, `--visibility`, `--fork`, `--source`, `--archived`, `--no-archived`, `-l`, `--topic`, `--json` |
| `eg repo view [repo]` | `--web`, `--branch`, `--json` |
| `eg repo clone <repo> [dir] [-- gitflags]` | aggiunge il remote `upstream` per i fork (`-u`, `--no-upstream`) |
| `eg repo create [nome]` | `--public/--private/--internal`, `-d`, `--clone`, `--source . --push`, `--remote`, `--add-readme`, `-g/--gitignore`, `-l/--license` |
| `eg repo fork [repo] [-- gitflags]` | `--clone`, `--remote`, `--remote-name`, `--org`, `--fork-name`; rinomina `origin` in `upstream` come gh |
| `eg repo gitignore list / view`, `eg repo license list / view` | template del server; `license view --web` |
| `eg repo delete [repo] [--yes]` | |
| `eg repo edit [repo]` | `--description`, `--default-branch`, `--visibility` |
| `eg repo rename [nuovo-nome]` | aggiorna il remote git corrispondente |
| `eg repo sync [fork]` | sincronizzazione locale dal remote (fast-forward o `--force`); con un fork come argomento, `merge-upstream` (solo fast-forward) |
| `eg browse [numero \| path[:riga] \| sha]` | `--branch`, `--commit`, `--settings`, `--releases`, `--actions`, `--no-browser` |
| `eg issue list` | `-s`, `-l`, `-a`, `-A`, `--mention`, `-m`, `-S/--search`, `-L`, `--json`, `--web` |
| `eg issue view <n>` | `--comments`, `--json`, `--web` |
| `eg issue create` | `-t`, `-b`, `-F`, `-e`, `-a`, `-l`, `-m`, `--web`; domande ed editor nel terminale |
| `eg issue edit <n>...` | `-t`, `-b`, `-F`, `--add/remove-label`, `--add/remove-assignee`, `-m`, `--remove-milestone` |
| `eg issue close / reopen <n>` | `-c`, `-r completed\|not planned\|duplicate`, `--duplicate-of` |
| `eg issue comment <n>` | `-b`, `-F`, `-e`, `--web`, `--edit-last`, `--delete-last`, `--create-if-none` |
| `eg issue status` | assegnate a te, che ti menzionano, aperte da te |
| `eg pr list` | `-s open\|closed\|merged\|all`, `-B`, `-H`, `-l`, `-a`, `-A`, `-d`, `-S`, `-L`, `--json`, `--web` |
| `eg pr view / diff [n\|url\|branch]` | `--comments`, `--json`, `--web`; diff `--patch`, `--name-only`, `--color` |
| `eg pr create` | `-t`, `-b`, `-F`, `-B`, `-H`, `-d`, `-f`, `--fill-first`, `-e`, `-a`, `-l`, `-m`, `-r`, `--web`, `--dry-run`, `--no-maintainer-edit`; fa il push del branch (chiede prima) |
| `eg pr checkout <n>` | `-b`, `-f`, `--detach`, `--recurse-submodules`; i fork passano da `refs/pull/<n>/head` |
| `eg pr merge` | `-m/-s/-r`, `-t`, `-b`, `-F`, `-d`, `--auto`, `--disable-auto`, `--match-head-commit` |
| `eg pr close / reopen / ready / comment / edit / review` | come gh; `ready --undo`, `close -d`, `edit -B --add/remove-reviewer` |
| `eg pr status / checks` | checks: status e check run, `--watch`, `--fail-fast`, `-i`, `--json`; uscita 8 se in attesa |
| `eg release list / view / create / edit / delete` | `--notes`, `-F`, `-t`, `--target`, `-d`, `-p`, `--verify-tag`, `--notes-from-tag`, `--cleanup-tag` |
| `eg release upload / download / delete-asset` | `--clobber`; download `-p`, `-D`, `-O`, `-A zip\|tar.gz`, `--skip-existing` |
| `eg label list / create / edit / delete` | `-S`, `--sort`, `--order`, `-c`, `-d`, `-f`, `-n` |
| `eg run list` | `-L`, `-w`, `-b`, `-u`, `-e`, `-s`, `-c`, `--created`, `--json` |
| `eg run view [id]` | `--log`, `--log-failed`, `-j`, `-v`, `-a`, `--exit-status`, `--web`, `--json`; le run importate da GitHub mostrano il link a GitHub |
| `eg run watch / rerun / cancel [id]` | watch `-i`, `--exit-status`, `--compact`; rerun solo di run intere |
| `eg run download [id]` | `-n`, `-p`, `-D`; artifact estratti dal loro zip |
| `eg workflow list / view / run` | view `-y`, `-r`, `--web`; run `-f`, `-F` (`@file`), `--ref`, `--json` |
| `eg secret list / set / delete` | livello repository; `set -b`, `-f .env`, `--no-store`; valori cifrati con la chiave del repository |
| `eg variable list / get / set / delete` | livello repository; `set -b`, `-f .env`; `--json` |
| `eg search repos / issues / prs / code` | i flag qualificatori di gh; issues, prs e code cercano in un solo repository (`--repo`, `repo:` o il clone corrente) |
| `eg alias set / list / delete` | segnaposto `$1`, `--clobber`, `-` legge l'espansione da stdin; niente alias di shell |
| `eg config get / set / list` | `git_protocol`, `editor`, `prompt`, `pager`, `browser`; `-h` per host |
| `eg completion -s bash\|zsh\|fish\|powershell` | completa comandi, flag e alias dall'albero dei comandi |

Il repository si risolve come in gh: `--repo/-R [HOST/]OWNER/REPO`, poi `EG_REPO`,
poi i remote git che puntano a un host su cui si è fatto login (`upstream`, poi
`origin`). `eg help environment` descrive `EG_HOST`, `EG_TOKEN`, `EG_REPO`,
`EG_CONFIG_DIR`, `EG_BROWSER`, `EG_PAGER`, `EG_EDITOR`, `EG_DEBUG`, `EG_FORCE_TTY`,
`EG_PROMPT_DISABLED`, `NO_COLOR`, `CLICOLOR_FORCE`.

Come gh, `eg` apre il browser (`EG_BROWSER`, chiave `browser` della config, `BROWSER`,
altrimenti `open`/`xdg-open`/`rundll32`), impagina l'output lungo nel terminale
(`EG_PAGER`, chiave `pager`, `PAGER`, altrimenti `less -R` con `LESS=FRX`) e apre un
editor per i testi (`EG_EDITOR`, chiave `editor`, `VISUAL`, `EDITOR`, altrimenti `nano`).
I valori sono divisi in parole come farebbe una shell, ma non passano mai da una shell.

Codici di uscita: 0 successo, 1 errore, 2 annullato, 4 autenticazione richiesta,
8 check in attesa.

## Differenze da gh

- Non c'è un host predefinito: `eg` usa `EG_HOST`, altrimenti il primo host su cui
  si è fatto login. `EG_TOKEN` vale per tutti gli host.
- `--jq` accetta solo percorsi semplici (`.name`, `.[].name`, `.items | .[0]`);
  `--template` non è supportato.
- I token stanno sempre in `hosts.yml` (niente keyring).
- Non ancora supportati dal server: `repo create --template`, le impostazioni di `repo edit` diverse
  da descrizione, branch predefinito e visibilità, `browse --wiki/--projects`, `issue delete` (manca la
  mutation `deleteIssue`), `issue lock/unlock` (manca `PUT/DELETE .../issues/{n}/lock`), `release create
  --generate-notes/--notes-start-tag` (manca `POST .../releases/generate-notes`), `release --latest`
  (`make_latest` viene ignorato), `pr merge --admin/--author-email`, `pr checks --required`, revisori
  team e project, `workflow enable/disable`, `run delete`, `run rerun --failed/--job/--debug`, secret e
  variabili di organizzazione, ambiente e utente (`--org`, `--env`, `--user`), `search commits`,
  `repo sync --force` remoto. `repo fork --default-branch-only` è accettato ma vengono copiati tutti i branch.
- `issue list`/`pr list` stampano "Showing N of M" contando le pagine REST (fino a 1000
  elementi); `--search`, `status` e `eg search issues/prs/code` richiedono un repository (`repo:`),
  che `eg` prende da `--repo` o dal clone corrente.
- Le run importate da GitHub (sincronizzazione GitHub) mostrano riepilogo e link a GitHub; job, log e
  artifact restano su GitHub.
- Gli alias si espandono nel processo: gli alias di shell (`!`, `--shell`) sono rifiutati perché `eg`
  non avvia mai una shell.
- Non ancora implementati: `issue develop/pin/unpin/transfer`, `label clone`, `gist`, `status` (il
  server non cerca su più repository), `auth switch/refresh`, `repo set-default`, `extension`, `cache`,
  `alias import`, i prompt interattivi degli input di `workflow run`.

## Sviluppo

`make build`, `make vet`, `make fmt-check`, `make lint`. Solo libreria standard, più
`golang.org/x/crypto` per la sealed box libsodium di `eg secret set`.
