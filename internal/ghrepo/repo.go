// Package ghrepo parses repository references: OWNER/REPO,
// HOST/OWNER/REPO and git remote URLs.
package ghrepo

import (
	"fmt"
	"net/url"
	"strings"
)

// Repo identifies a repository on a host.
type Repo struct {
	Host  string
	Owner string
	Name  string
}

// New returns a repository on host.
func New(host, owner, name string) Repo {
	return Repo{Host: NormalizeHost(host), Owner: owner, Name: strings.TrimSuffix(name, ".git")}
}

// FullName is OWNER/REPO.
func (r Repo) FullName() string { return r.Owner + "/" + r.Name }

// String is HOST/OWNER/REPO.
func (r Repo) String() string { return r.Host + "/" + r.FullName() }

// Equal compares two repositories ignoring case.
func (r Repo) Equal(o Repo) bool {
	return strings.EqualFold(r.Host, o.Host) && strings.EqualFold(r.Owner, o.Owner) && strings.EqualFold(r.Name, o.Name)
}

// NormalizeHost lowercases a host and drops a leading "www.".
func NormalizeHost(h string) string { return strings.TrimPrefix(strings.ToLower(h), "www.") }

// Hostname strips the port of a host.
func Hostname(h string) string {
	if i := strings.LastIndexByte(h, ':'); i >= 0 && !strings.Contains(h[i:], "]") {
		return h[:i]
	}
	return h
}

// FromFullName parses OWNER/REPO, HOST/OWNER/REPO or a URL; defaultHost
// fills a missing host.
func FromFullName(s, defaultHost string) (Repo, error) {
	if IsURL(s) {
		u, err := ParseURL(s)
		if err != nil {
			return Repo{}, err
		}
		return FromURL(u)
	}
	parts := strings.Split(s, "/")
	for _, p := range parts {
		if p == "" {
			return Repo{}, fmt.Errorf(`expected the "[HOST/]OWNER/REPO" format, got %q`, s)
		}
	}
	switch len(parts) {
	case 3:
		return New(parts[0], parts[1], parts[2]), nil
	case 2:
		if defaultHost == "" {
			return Repo{}, fmt.Errorf("could not determine the host of %q; use HOST/OWNER/REPO or set EG_HOST", s)
		}
		return New(defaultHost, parts[0], parts[1]), nil
	}
	return Repo{}, fmt.Errorf(`expected the "[HOST/]OWNER/REPO" format, got %q`, s)
}

// FromURL extracts the repository of an https or ssh URL.
func FromURL(u *url.URL) (Repo, error) {
	if u.Hostname() == "" {
		return Repo{}, fmt.Errorf("no hostname detected")
	}
	parts := strings.SplitN(strings.Trim(u.Path, "/"), "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return Repo{}, fmt.Errorf("invalid path: %s", u.Path)
	}
	host := u.Hostname()
	// The port of an https URL is part of the host; the port of an ssh URL
	// belongs to the ssh server.
	if u.Port() != "" && (u.Scheme == "https" || u.Scheme == "http") {
		host = u.Host
	}
	return New(host, parts[0], parts[1]), nil
}

// IsURL reports whether s looks like a git or web URL.
func IsURL(s string) bool {
	return strings.Contains(s, "://") || isSCPLike(s)
}

func isSCPLike(s string) bool {
	at := strings.Index(s, "@")
	colon := strings.Index(s, ":")
	return at > 0 && colon > at && !strings.Contains(s[:colon], "/")
}

// ParseURL parses a URL, including the scp-like git@host:owner/repo form.
func ParseURL(raw string) (*url.URL, error) {
	if isSCPLike(raw) && !strings.Contains(raw, "://") {
		raw = "ssh://" + strings.Replace(raw, ":", "/", 1)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse %q: %w", raw, err)
	}
	switch u.Scheme {
	case "git+https":
		u.Scheme = "https"
	case "git+ssh":
		u.Scheme = "ssh"
	}
	return u, nil
}

// CloneURL returns the git URL of r for protocol "https" or "ssh"; scheme
// is the web scheme of the host ("http" for local servers).
func CloneURL(r Repo, protocol, scheme string) string {
	if protocol == "ssh" {
		return "git@" + Hostname(r.Host) + ":" + r.FullName() + ".git"
	}
	if scheme == "" {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/" + r.FullName() + ".git"
}

// WebURL is the page of r, plus an optional path.
func WebURL(r Repo, scheme, p string) string {
	if scheme == "" {
		scheme = "https"
	}
	u := scheme + "://" + r.Host + "/" + r.FullName()
	if p != "" {
		u += "/" + strings.TrimPrefix(p, "/")
	}
	return u
}
