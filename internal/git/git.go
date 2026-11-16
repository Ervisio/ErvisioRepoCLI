// Package git runs the git binary. It is the only place that starts
// processes: argv is passed to git directly, never through a shell.
package git

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// ErrNotRepo is returned outside a git work tree.
var ErrNotRepo = errors.New("not a git repository")

// Client runs git in Dir (the current directory when empty).
type Client struct {
	Dir    string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Error is a failed git command.
type Error struct {
	Args     []string
	ExitCode int
	Stderr   string
}

func (e *Error) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		msg = fmt.Sprintf("exit status %d", e.ExitCode)
	}
	return fmt.Sprintf("git %s: %s", firstArg(e.Args), msg)
}

func firstArg(a []string) string {
	if len(a) == 0 {
		return ""
	}
	return a[0]
}

// command is the single helper that builds a git process.
func (c *Client) command(args ...string) (*exec.Cmd, error) {
	bin, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("unable to find git executable in PATH; please install Git before retrying: %w", err)
	}
	cmd := exec.Command(bin, args...) //nolint:gosec // argv is passed as is, never through a shell
	cmd.Dir = c.Dir
	return cmd, nil
}

// Output runs git and returns its stdout.
func (c *Client) Output(args ...string) (string, error) {
	cmd, err := c.command(args...)
	if err != nil {
		return "", err
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return out.String(), wrap(args, err, errb.String())
	}
	return out.String(), nil
}

// Run runs git attached to the client streams (progress shown to the
// user).
func (c *Client) Run(args ...string) error {
	cmd, err := c.command(args...)
	if err != nil {
		return err
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = c.Stdin, c.Stdout, c.Stderr
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		return wrap(args, err, "")
	}
	return nil
}

func wrap(args []string, err error, stderr string) error {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return &Error{Args: args, ExitCode: ee.ExitCode(), Stderr: stderr}
	}
	return fmt.Errorf("run git %s: %w", firstArg(args), err)
}

// GitDir returns the .git directory, or ErrNotRepo.
func (c *Client) GitDir() (string, error) {
	out, err := c.Output("rev-parse", "--git-dir")
	if err != nil {
		var ge *Error
		if errors.As(err, &ge) {
			return "", ErrNotRepo
		}
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Remote is a git remote with its fetch URL.
type Remote struct {
	Name     string
	FetchURL string
	PushURL  string
	Resolved string
}

// Remotes lists the remotes, ordered upstream, github, origin, then by
// name as gh does.
func (c *Client) Remotes() ([]Remote, error) {
	out, err := c.Output("remote", "-v")
	if err != nil {
		var ge *Error
		if errors.As(err, &ge) {
			return nil, ErrNotRepo
		}
		return nil, err
	}
	byName := map[string]*Remote{}
	var list []*Remote
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) < 3 {
			continue
		}
		r := byName[f[0]]
		if r == nil {
			r = &Remote{Name: f[0]}
			byName[f[0]] = r
			list = append(list, r)
		}
		if f[2] == "(push)" {
			r.PushURL = f[1]
		} else {
			r.FetchURL = f[1]
		}
	}
	if res, err := c.Output("config", "--get-regexp", `^remote\..*\.eg-resolved$`); err == nil {
		for _, l := range strings.Split(res, "\n") {
			k, v, ok := strings.Cut(l, " ")
			if !ok {
				continue
			}
			name := strings.TrimSuffix(strings.TrimPrefix(k, "remote."), ".eg-resolved")
			if r := byName[name]; r != nil {
				r.Resolved = strings.TrimSpace(v)
			}
		}
	}
	rank := func(n string) int {
		switch n {
		case "upstream":
			return 0
		case "github", "ervisio":
			return 1
		case "origin":
			return 2
		}
		return 3
	}
	sort.SliceStable(list, func(i, j int) bool {
		ri, rj := rank(list[i].Name), rank(list[j].Name)
		if ri != rj {
			return ri < rj
		}
		return list[i].Name < list[j].Name
	})
	res := make([]Remote, len(list))
	for i, r := range list {
		res[i] = *r
	}
	return res, nil
}

// CurrentBranch returns the checked-out branch.
func (c *Client) CurrentBranch() (string, error) {
	out, err := c.Output("symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return "", fmt.Errorf("could not determine current branch: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// ConfigGet reads one git config value ("" when unset).
func (c *Client) ConfigGet(key string) string {
	out, err := c.Output("config", "--get", key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// ToplevelDir returns the root of the work tree.
func (c *Client) ToplevelDir() (string, error) {
	out, err := c.Output("rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}
