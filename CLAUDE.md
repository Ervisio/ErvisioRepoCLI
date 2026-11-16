# ErvisioRepoCLI — regole per gli agenti

`eg` è il client CLI di ErvisioRepo e deve comportarsi come `gh` (comandi, flag,
output, codici di uscita; variabili d'ambiente `EG_*`).

1. Solo libreria standard Go (`flag`, `net/http`, `encoding/json`...); nessuna dipendenza
   esterna. Non si modifica `go.mod` senza dirlo nel report.
2. `os/exec` solo in `internal/git` (per `git`) e in `internal/launch` (browser, pager,
   editor dell'utente, come gh), sempre con argv diretto e mai via shell. I valori di
   `EG_BROWSER`/`BROWSER`, `EG_PAGER`/`PAGER` e `EG_EDITOR`/`VISUAL`/`EDITOR` (o delle chiavi
   `browser`, `pager`, `editor` di `config.yml`) si dividono in argv con `launch.SplitWords`.
   Browser predefinito: `open` (macOS), `xdg-open` (Linux), `rundll32 url.dll,FileProtocolHandler`
   (Windows); pager predefinito `less -R` con `LESS=FRX` solo se stdout è un terminale.
3. Prima di implementare un comando si verifica che la route esista nel server
   (`internal/ghapi/ghapi.go` di ErvisioRepo); se manca, errore chiaro
   "not supported by this ErvisioRepo server yet" (`cmdutil.Unsupported`), mai simulare.
4. **Niente test**: non si scrivono né si eseguono test, niente CI (nessun workflow GitHub).
   Le verifiche sono `go build ./...`, `go vet ./...`, `gofmt -l .` vuoto e `make lint`.
5. Codice, commenti e messaggi in inglese; documentazione utente in inglese (`README.md`)
   e italiano (`README.it.md`). Errori Go con `fmt.Errorf("…: %w", err)`.
6. Ogni task aggiorna `CHANGELOG.md` (`## [Unreleased]`) e si chiude con un commit
   Conventional Commits direttamente su `main`, poi `git push origin main`;
   mai `--no-verify`, mai force-push.

## Struttura

- `cmd/eg`: `main` minimale.
- `internal/cmd/<gruppo>`: comandi (`auth`, `api`, `repo`, `browse`, `issue`, `pr`,
  `release`, `label`, `root`); `internal/cmd/shared` contiene tipi e helper comuni a issue e pr.
- `internal/cmdutil`: albero dei comandi, flag, help, factory, tabelle, `--json`, prompt.
- `internal/api`: client REST/GraphQL; `internal/config`: `hosts.yml`/`config.yml`;
  `internal/ghrepo`: parsing dei repository; `internal/git`: esecuzione di git;
  `internal/iostreams`: TTY e colori; `internal/jsonx`: JSON colorato e `--jq` semplice;
  `internal/launch`: browser, pager ed editor.
