package shared

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
)

// ParseDotenv reads KEY=VALUE lines as gh secret/variable set -f does:
// blank lines and # comments are skipped, "export " is allowed and
// values may be single or double quoted.
func ParseDotenv(r io.Reader) ([]string, map[string]string, error) {
	var keys []string
	out := map[string]string{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			return nil, nil, fmt.Errorf("line %d: expected KEY=VALUE", n)
		}
		v = strings.TrimSpace(v)
		switch {
		case len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"':
			u, err := strconv.Unquote(v)
			if err != nil {
				return nil, nil, fmt.Errorf("line %d: %w", n, err)
			}
			v = u
		case len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'':
			v = v[1 : len(v)-1]
		default:
			if i := strings.Index(v, " #"); i >= 0 {
				v = strings.TrimSpace(v[:i])
			}
		}
		if _, dup := out[k]; !dup {
			keys = append(keys, k)
		}
		out[k] = v
	}
	if err := sc.Err(); err != nil {
		return nil, nil, fmt.Errorf("read env file: %w", err)
	}
	return keys, out, nil
}

// ScopeFlags holds gh's --org/--env/--user/--app flags of secret
// and variable commands; this server only has repository scope.
type ScopeFlags struct {
	Org, Env, App string
	User          bool
}

// Register adds the flags.
func (s *ScopeFlags) Register(fl *cmdutil.FlagSet, user, app bool) {
	fl.StringVarP(&s.Org, "org", "o", "", "Set `organization` (not supported by ErvisioRepo)")
	fl.StringVarP(&s.Env, "env", "e", "", "Set deployment `environment` (not supported by ErvisioRepo)")
	if user {
		fl.BoolVarP(&s.User, "user", "u", false, "Set a secret for your user (not supported by ErvisioRepo)")
	}
	if app {
		fl.StringVarP(&s.App, "app", "a", "", "Set the application for a secret: {actions}")
	}
}

// Check refuses scopes other than the repository.
func (s *ScopeFlags) Check(noun string) error {
	switch {
	case s.Org != "":
		return cmdutil.Unsupported("Organization "+noun, "/orgs/{org}/actions/"+noun)
	case s.Env != "":
		return cmdutil.Unsupported("Environment "+noun, "/repos/{owner}/{repo}/environments/{env}/"+noun)
	case s.User:
		return cmdutil.Unsupported("User (Codespaces) "+noun, "/user/codespaces/secrets")
	case s.App != "" && !strings.EqualFold(s.App, "actions"):
		return cmdutil.Unsupported("The "+s.App+" application", "")
	}
	return nil
}
