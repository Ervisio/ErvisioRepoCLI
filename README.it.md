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
monouso e l'indirizzo di `https://<host>/login/device`; aprilo, accedi e inserisci
il codice. I personal access token (prefisso `erp_`) si creano dall'interfaccia web
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
| `eg repo clone <repo> [dir] [-- gitflags]` | aggiunge il remote `upstream` per i fork |
| `eg repo create [nome]` | `--public/--private/--internal`, `-d`, `--clone`, `--source . --push`, `--remote` |
| `eg repo delete [repo] [--yes]` | |
| `eg repo edit [repo]` | `--description`, `--default-branch`, `--visibility` |
| `eg repo rename [nuovo-nome]` | aggiorna il remote git corrispondente |
| `eg repo sync` | sincronizzazione locale dal remote (fast-forward o `--force`) |
| `eg browse [numero \| path[:riga] \| sha]` | `--branch`, `--commit`, `--settings`, `--releases`, `--actions` |

Il repository si risolve come in gh: `--repo/-R [HOST/]OWNER/REPO`, poi `EG_REPO`,
poi i remote git che puntano a un host su cui si è fatto login (`upstream`, poi
`origin`). `eg help environment` descrive `EG_HOST`, `EG_TOKEN`, `EG_REPO`,
`EG_CONFIG_DIR`, `EG_DEBUG`, `EG_FORCE_TTY`, `EG_PROMPT_DISABLED`, `NO_COLOR`.

Codici di uscita: 0 successo, 1 errore, 2 annullato, 4 autenticazione richiesta.

## Differenze da gh

- Non c'è un host predefinito: `eg` usa `EG_HOST`, altrimenti il primo host su cui
  si è fatto login. `EG_TOKEN` vale per tutti gli host.
- `eg` non avvia browser né pager (l'unico processo che lancia è `git`): `--web`,
  `eg browse` e il login stampano l'indirizzo da aprire.
- `--jq` accetta solo percorsi semplici (`.name`, `.[].name`, `.items | .[0]`);
  `--template` non è supportato.
- I token stanno sempre in `hosts.yml` (niente keyring).
- Non ancora supportati dal server: `repo fork` (manca `POST /repos/{owner}/{repo}/forks`),
  `repo sync` remoto (manca `merge-upstream`), `repo create --template/--gitignore/--license/--add-readme`,
  le impostazioni di `repo edit` diverse da descrizione, branch predefinito e visibilità,
  `browse --wiki/--projects`.
- Non ancora implementati: `issue`, `pr`, `release`, `run`, `workflow`, `search`,
  `gist`, `status`, `auth switch/refresh`, `repo set-default`, `config`, `alias`, `extension`.

## Sviluppo

`make build`, `make vet`, `make fmt-check`, `make lint`. Solo libreria standard.
