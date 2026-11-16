package cmdutil

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/config"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
	"github.com/Ervisio/ErvisioRepoCLI/internal/git"
	"github.com/Ervisio/ErvisioRepoCLI/internal/iostreams"
)

// Factory gives commands their dependencies.
type Factory struct {
	IO *iostreams.IOStreams
	// Executable is the path of the running eg binary.
	Executable string

	cfgOnce sync.Once
	cfg     *config.Config
	cfgErr  error
}

// NewFactory builds the factory of the process.
func NewFactory() *Factory {
	exe, err := os.Executable()
	if err != nil {
		exe = "eg"
	}
	return &Factory{IO: iostreams.System(), Executable: exe}
}

// Config loads the configuration once.
func (f *Factory) Config() (*config.Config, error) {
	f.cfgOnce.Do(func() { f.cfg, f.cfgErr = config.Load() })
	return f.cfg, f.cfgErr
}

// Git returns a git client attached to the command streams.
func (f *Factory) Git(dir string) *git.Client {
	return &git.Client{Dir: dir, Stdin: f.IO.In, Stdout: f.IO.Out, Stderr: f.IO.ErrOut}
}

// DefaultHost returns the host used outside a repository.
func (f *Factory) DefaultHost() (string, error) {
	cfg, err := f.Config()
	if err != nil {
		return "", err
	}
	h, _ := cfg.DefaultHost()
	if h == "" {
		return "", &AuthError{}
	}
	return h, nil
}

// Client returns an authenticated API client for host.
func (f *Factory) Client(host string) (*api.Client, error) {
	cfg, err := f.Config()
	if err != nil {
		return nil, err
	}
	tok, _ := cfg.Token(host)
	if tok == "" {
		if len(cfg.Hosts()) == 0 {
			return nil, &AuthError{}
		}
		return nil, &AuthError{Host: host}
	}
	return api.New(host, cfg.Scheme(host), tok), nil
}

// BaseRepo resolves the repository a command works on: the --repo value,
// $EG_REPO, then the git remotes that point to a logged-in host.
func (f *Factory) BaseRepo(override string) (ghrepo.Repo, error) {
	cfg, err := f.Config()
	if err != nil {
		return ghrepo.Repo{}, err
	}
	if override == "" {
		override = os.Getenv("EG_REPO")
	}
	if override != "" {
		def, _ := cfg.DefaultHost()
		r, err := ghrepo.FromFullName(override, def)
		if err != nil {
			return ghrepo.Repo{}, err
		}
		if h := MatchHost(cfg.Hosts(), r.Host); h != "" {
			r.Host = h
		}
		return r, nil
	}
	remotes, err := f.Git("").Remotes()
	if errors.Is(err, git.ErrNotRepo) {
		return ghrepo.Repo{}, errors.New("not a git repository (or any of the parent directories); use --repo [HOST/]OWNER/REPO")
	}
	if err != nil {
		return ghrepo.Repo{}, fmt.Errorf("could not determine the repository: %w", err)
	}
	hosts := cfg.Hosts()
	if len(hosts) == 0 {
		return ghrepo.Repo{}, &AuthError{}
	}
	var found []ghrepo.Repo
	for _, rm := range remotes {
		r, ok := RemoteRepo(hosts, rm.FetchURL)
		if !ok {
			continue
		}
		if rm.Resolved == "base" {
			return r, nil
		}
		found = append(found, r)
	}
	if len(found) == 0 {
		return ghrepo.Repo{}, errors.New("none of the git remotes configured for this repository point to a known ErvisioRepo host. " +
			"To tell eg about a new host, please use `eg auth login`")
	}
	return found[0], nil
}

// RemoteRepo maps a remote URL to a repository on one of hosts.
func RemoteRepo(hosts []string, rawURL string) (ghrepo.Repo, bool) {
	u, err := ghrepo.ParseURL(rawURL)
	if err != nil {
		return ghrepo.Repo{}, false
	}
	r, err := ghrepo.FromURL(u)
	if err != nil {
		return ghrepo.Repo{}, false
	}
	h := MatchHost(hosts, r.Host)
	if h == "" {
		return ghrepo.Repo{}, false
	}
	r.Host = h
	return r, true
}

// MatchHost finds host among the configured hosts, first exactly, then
// ignoring the port (an ssh remote has no web port).
func MatchHost(hosts []string, host string) string {
	host = ghrepo.NormalizeHost(host)
	for _, h := range hosts {
		if strings.EqualFold(h, host) {
			return h
		}
	}
	for _, h := range hosts {
		if strings.EqualFold(ghrepo.Hostname(h), ghrepo.Hostname(host)) {
			return h
		}
	}
	return ""
}

// WebURL returns the page of r with the scheme of its host.
func (f *Factory) WebURL(r ghrepo.Repo, p string) string {
	scheme := "https"
	if cfg, err := f.Config(); err == nil {
		scheme = cfg.Scheme(r.Host)
	}
	return ghrepo.WebURL(r, scheme, p)
}

// OpenURL stands in for opening a browser: eg starts no process other
// than git, so it prints the URL to open (like gh's --no-browser).
func (f *Factory) OpenURL(u string) error {
	if f.IO.IsStderrTTY() {
		display := u
		if p, err := url.Parse(u); err == nil {
			display = p.Host + p.Path
		}
		fmt.Fprintf(f.IO.ErrOut, "Open %s in your browser.\n", display)
	}
	_, err := fmt.Fprintln(f.IO.Out, u)
	return err
}
