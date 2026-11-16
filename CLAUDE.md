# ErvisioRepoCLI — regole per gli agenti

`eg` è il client CLI di ErvisioRepo e deve comportarsi come `gh` (comandi, flag,
output, codici di uscita; variabili d'ambiente `EG_*`).

1. Solo libreria standard Go (`flag`, `net/http`, `encoding/json`...); nessuna dipendenza
   esterna. Non si modifica `go.mod` senza dirlo nel report.
2. `os/exec` solo per lanciare `git`, e solo da `internal/git` (argv diretto, mai shell).
   Niente browser né pager: si stampa l'URL.
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
- `internal/cmd/<gruppo>`: comandi (`auth`, `api`, `repo`, `browse`, `root`).
- `internal/cmdutil`: albero dei comandi, flag, help, factory, tabelle, `--json`, prompt.
- `internal/api`: client REST/GraphQL; `internal/config`: `hosts.yml`/`config.yml`;
  `internal/ghrepo`: parsing dei repository; `internal/git`: esecuzione di git;
  `internal/iostreams`: TTY e colori; `internal/jsonx`: JSON colorato e `--jq` semplice.
