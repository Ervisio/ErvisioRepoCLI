// Package config reads and writes ~/.config/eg/{config.yml,hosts.yml}
// in the same shape as gh.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	hostsFile  = "hosts.yml"
	configFile = "config.yml"
)

// Config is the loaded configuration.
type Config struct {
	dir   string
	main  *Node
	hosts *Node
}

// Dir returns the configuration directory: $EG_CONFIG_DIR,
// $XDG_CONFIG_HOME/eg, %AppData%\eg on Windows, or ~/.config/eg.
func Dir() string {
	if d := os.Getenv("EG_CONFIG_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "eg")
	}
	if runtime.GOOS == "windows" {
		if d := os.Getenv("AppData"); d != "" {
			return filepath.Join(d, "eg")
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "eg")
	}
	return filepath.Join(home, ".config", "eg")
}

// Load reads both files; missing files are empty.
func Load() (*Config, error) {
	c := &Config{dir: Dir()}
	var err error
	if c.main, err = readYAML(filepath.Join(c.dir, configFile)); err != nil {
		return nil, err
	}
	if c.hosts, err = readYAML(filepath.Join(c.dir, hostsFile)); err != nil {
		return nil, err
	}
	return c, nil
}

func readYAML(path string) (*Node, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return NewMap(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	n, err := ParseYAML(data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return n, nil
}

// HostsPath is the path of hosts.yml.
func (c *Config) HostsPath() string { return filepath.Join(c.dir, hostsFile) }

// Get returns a key of config.yml, overridden per host by hosts.yml.
func (c *Config) Get(host, key string) string {
	if host != "" {
		if v := c.hosts.String(host, key); v != "" {
			return v
		}
	}
	return c.main.String(key)
}

// Hosts lists the configured hosts, plus $EG_HOST when $EG_TOKEN is set.
func (c *Config) Hosts() []string {
	hosts := c.hosts.Keys()
	if h, _ := envHost(); h != "" && os.Getenv("EG_TOKEN") != "" && !contains(hosts, h) {
		hosts = append([]string{h}, hosts...)
	}
	return hosts
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// envHost parses $EG_HOST, which may carry a scheme.
func envHost() (string, string) {
	v := os.Getenv("EG_HOST")
	if v == "" {
		return "", ""
	}
	h, scheme, err := ParseHostInput(v)
	if err != nil {
		return "", ""
	}
	return h, scheme
}

// ParseHostInput normalizes "host", "host:port" or "http(s)://host[:port]"
// into the host key and the API scheme ("" when not given).
func ParseHostInput(v string) (string, string, error) {
	v = strings.TrimSpace(v)
	scheme := ""
	if strings.Contains(v, "://") {
		u, err := url.Parse(v)
		if err != nil {
			return "", "", fmt.Errorf("invalid hostname %q: %w", v, err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return "", "", fmt.Errorf("invalid hostname %q: scheme must be http or https", v)
		}
		scheme, v = u.Scheme, u.Host
	}
	v = strings.ToLower(strings.TrimSuffix(v, "/"))
	if v == "" || strings.ContainsAny(v, "/ @") {
		return "", "", fmt.Errorf("invalid hostname %q", v)
	}
	return v, scheme, nil
}

// DefaultHost is the host used outside a repository: $EG_HOST, else the
// first configured host. The second value names where it came from.
func (c *Config) DefaultHost() (string, string) {
	if h, _ := envHost(); h != "" {
		return h, "EG_HOST"
	}
	if keys := c.hosts.Keys(); len(keys) > 0 {
		return keys[0], c.HostsPath()
	}
	return "", ""
}

// Token returns the token of host and its source.
func (c *Config) Token(host string) (string, string) {
	if t := os.Getenv("EG_TOKEN"); t != "" {
		return t, "EG_TOKEN"
	}
	if t := c.hosts.String(host, "oauth_token"); t != "" {
		return t, c.HostsPath()
	}
	return "", ""
}

// UserToken returns the stored token of one account of host.
func (c *Config) UserToken(host, user string) string {
	if t := c.hosts.String(host, "users", user, "oauth_token"); t != "" {
		return t
	}
	if c.User(host) == user {
		return c.hosts.String(host, "oauth_token")
	}
	return ""
}

// User returns the active account of host.
func (c *Config) User(host string) string { return c.hosts.String(host, "user") }

// Users lists the accounts stored for host.
func (c *Config) Users(host string) []string {
	users := c.hosts.Path(host, "users").Keys()
	if len(users) == 0 {
		if u := c.User(host); u != "" {
			users = []string{u}
		}
	}
	return users
}

// GitProtocol returns the git protocol of host: hosts.yml, config.yml,
// then "https".
func (c *Config) GitProtocol(host string) string {
	if p := c.Get(host, "git_protocol"); p != "" {
		return p
	}
	return "https"
}

// Scheme returns the scheme used to reach host: "http" only when the host
// was logged in with an http:// URL (local servers) or $EG_HOST says so.
func (c *Config) Scheme(host string) string {
	if h, s := envHost(); h == host && s != "" {
		return s
	}
	if s := c.hosts.String(host, "api_protocol"); s == "http" {
		return s
	}
	return "https"
}

// Login stores the account of host and makes it active.
func (c *Config) Login(host, user, token, gitProtocol, scheme string) error {
	h := c.hosts.Map(host)
	if user != "" {
		h.Map("users").Map(user).SetString("oauth_token", token)
		h.SetString("user", user)
	}
	h.SetString("oauth_token", token)
	if gitProtocol != "" {
		h.SetString("git_protocol", gitProtocol)
	}
	switch scheme {
	case "http":
		h.SetString("api_protocol", "http")
	case "https":
		h.Delete("api_protocol")
	}
	return c.writeHosts()
}

// Logout removes user from host (every account when user is empty). When
// another account remains it becomes active.
func (c *Config) Logout(host, user string) error {
	h := c.hosts.Get(host)
	if h == nil {
		return fmt.Errorf("not logged in to %s", host)
	}
	users := h.Map("users")
	if user != "" {
		users.Delete(user)
	}
	if user == "" || len(users.Keys()) == 0 {
		c.hosts.Delete(host)
		return c.writeHosts()
	}
	if h.String("user") == user {
		next := users.Keys()[0]
		h.SetString("user", next)
		h.SetString("oauth_token", users.String(next, "oauth_token"))
	}
	return c.writeHosts()
}

// SetGitProtocol stores the git protocol of host.
func (c *Config) SetGitProtocol(host, p string) error {
	c.hosts.Map(host).SetString("git_protocol", p)
	return c.writeHosts()
}

func (c *Config) writeHosts() error { return c.writeFile(c.HostsPath(), c.hosts) }

func (c *Config) writeMain() error { return c.writeFile(filepath.Join(c.dir, configFile), c.main) }

// writeFile replaces path atomically with the encoding of n (mode 0600).
func (c *Config) writeFile(path string, n *Node) error {
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	tmp, err := os.CreateTemp(c.dir, ".eg-*.yml")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // gone after the rename
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close() //nolint:errcheck,gosec // already failing
		return fmt.Errorf("write %s: %w", path, err)
	}
	if _, err := tmp.Write(n.Encode()); err != nil {
		tmp.Close() //nolint:errcheck,gosec // already failing
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// Set stores a key in config.yml, or in the block of host in hosts.yml.
func (c *Config) Set(host, key, value string) error {
	if host != "" {
		c.hosts.Map(host).SetString(key, value)
		return c.writeHosts()
	}
	c.main.SetString(key, value)
	return c.writeMain()
}

// Aliases returns the aliases of config.yml in file order.
func (c *Config) Aliases() ([]string, map[string]string) {
	a := c.main.Get("aliases")
	names := a.Keys()
	out := make(map[string]string, len(names))
	for _, n := range names {
		out[n] = a.String(n)
	}
	return names, out
}

// SetAlias stores an alias.
func (c *Config) SetAlias(name, expansion string) error {
	c.main.Map("aliases").SetString(name, expansion)
	return c.writeMain()
}

// DeleteAlias removes an alias.
func (c *Config) DeleteAlias(name string) error {
	c.main.Map("aliases").Delete(name)
	return c.writeMain()
}
