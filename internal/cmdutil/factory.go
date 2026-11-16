package cmdutil

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/config"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
	"github.com/Ervisio/ErvisioRepoCLI/internal/git"
	"github.com/Ervisio/ErvisioRepoCLI/internal/iostreams"
	"github.com/Ervisio/ErvisioRepoCLI/internal/launch"
)

// Factory gives commands their dependencies.
type Factory struct {
	IO *iostreams.IOStreams
	// Executable is the path of the running eg binary.
	Executable string

	cfgOnce sync.Once
	cfg     *config.Config
	cfgErr  error

	pager    *launch.Pager
	pagerOut io.Writer
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

// configValue returns a key of config.yml ("" when unset or unreadable).
func (f *Factory) configValue(key string) string {
	cfg, err := f.Config()
	if err != nil {
		return ""
	}
	return cfg.Get("", key)
}

// BrowserCommand is the configured browser: $EG_BROWSER, the browser key
// of config.yml, then $BROWSER ("" means the platform opener).
func (f *Factory) BrowserCommand() string {
	if v := os.Getenv("EG_BROWSER"); v != "" {
		return v
	}
	if v := f.configValue("browser"); v != "" {
		return v
	}
	return os.Getenv("BROWSER")
}

// PagerCommand is the configured pager: $EG_PAGER, the pager key of
// config.yml, then $PAGER, else "less -R". "cat" or an empty value turns
// paging off.
func (f *Factory) PagerCommand() string {
	for _, v := range []string{os.Getenv("EG_PAGER"), f.configValue("pager"), os.Getenv("PAGER")} {
		if v != "" {
			return v
		}
	}
	if _, set := os.LookupEnv("EG_PAGER"); set {
		return ""
	}
	return "less -R"
}

// EditorCommand is the configured editor: $EG_EDITOR, the editor key of
// config.yml, $VISUAL, $EDITOR, then nano (notepad on Windows).
func (f *Factory) EditorCommand() string {
	for _, v := range []string{os.Getenv("EG_EDITOR"), f.configValue("editor"), os.Getenv("VISUAL"), os.Getenv("EDITOR")} {
		if v != "" {
			return v
		}
	}
	return launch.DefaultEditor()
}

// Edit opens text in the editor and returns the result.
func (f *Factory) Edit(fileName, text string) (string, error) {
	return launch.Edit(f.EditorCommand(), fileName, text)
}

// OpenURL opens u in the browser, as gh does. It prints a short notice
// on a terminal.
func (f *Factory) OpenURL(u string) error {
	if f.IO.IsStderrTTY() {
		display := u
		if p, err := url.Parse(u); err == nil {
			display = p.Host + p.Path
		}
		fmt.Fprintf(f.IO.ErrOut, "Opening %s in your browser.\n", display)
	}
	return launch.Browse(f.BrowserCommand(), u, f.IO.Out, f.IO.ErrOut)
}

// StartPager pipes stdout through the pager when stdout is a terminal.
// Call StopPager when the output is complete.
func (f *Factory) StartPager() error {
	if !f.IO.IsStdoutTTY() || f.pager != nil {
		return nil
	}
	cmd := f.PagerCommand()
	if cmd == "" || cmd == "cat" {
		return nil
	}
	p, err := launch.StartPager(cmd, f.IO.Out, f.IO.ErrOut)
	if err != nil {
		return err
	}
	f.pager, f.pagerOut = p, f.IO.Out
	f.IO.Out = p
	return nil
}

// StopPager waits for the pager started by StartPager.
func (f *Factory) StopPager() {
	if f.pager == nil {
		return
	}
	_ = f.pager.Close()
	f.IO.Out = f.pagerOut
	f.pager, f.pagerOut = nil, nil
}
