// Package auth implements `eg auth`.
package auth

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
)

// New returns the auth command group.
func New(f *cmdutil.Factory) *cmdutil.Command {
	c := &cmdutil.Command{
		Use:     "auth <command>",
		Short:   "Authenticate eg and git with ErvisioRepo",
		GroupID: "core",
	}
	c.AddCommand(newLogin(f), newLogout(f), newStatus(f), newToken(f), newSetupGit(f), newGitCredential(f))
	return c
}

// requiredScopes are the classic scopes eg needs; the server maps
// repo:write to "repo" and user:read to "read:org".
var requiredScopes = []struct{ github, ervisio string }{
	{"repo", "repo:write"},
	{"read:org", "user:read"},
}

// whoami returns the login of the token owner and its scopes header.
func whoami(c *api.Client) (string, string, error) {
	resp, err := c.Request(http.MethodGet, c.RESTURL("user"), nil, nil)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body
	if resp.StatusCode >= 300 {
		return "", "", api.HandleHTTPError(resp)
	}
	var u struct {
		Login string `json:"login"`
	}
	if err := decode(resp, &u); err != nil {
		return "", "", err
	}
	return u.Login, resp.Header.Get("X-Oauth-Scopes"), nil
}

// checkScopes reports a missing required scope. Tokens without the
// header (fine-grained) pass, as with gh.
func checkScopes(header string) error {
	if strings.TrimSpace(header) == "" {
		return nil
	}
	have := map[string]bool{}
	for _, s := range strings.Split(header, ",") {
		have[strings.TrimSpace(s)] = true
	}
	for _, r := range requiredScopes {
		orgAlt := r.github == "read:org" && (have["admin:org"] || have["write:org"])
		if !have[r.github] && !orgAlt {
			return fmt.Errorf("missing required scope '%s' (create a token with the %s scope)", r.github, r.ervisio)
		}
	}
	return nil
}

func scopeList(header string) string {
	var out []string
	for _, s := range strings.Split(header, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, "'"+s+"'")
		}
	}
	if len(out) == 0 {
		return "none"
	}
	return strings.Join(out, ", ")
}

func tokensURL(scheme, host string) string {
	return scheme + "://" + host + "/settings?section=tokens"
}
