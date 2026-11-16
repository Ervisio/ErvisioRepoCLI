// Package launch starts the user's browser, pager and editor, as gh does.
// Commands are always started from an argv (a configured value is split
// into words, never handed to a shell). Together with internal/git it is
// the only package that starts processes.
package launch

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// SplitWords splits a command line into words the way a POSIX shell
// would for a simple command: blanks separate words, single quotes keep
// their content literally, double quotes allow \" \\ \$ and \` escapes,
// and a backslash outside quotes escapes the next character. There is no
// expansion of any kind.
func SplitWords(s string) ([]string, error) {
	var (
		words  []string
		cur    strings.Builder
		inWord bool
	)
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch r {
		case ' ', '\t', '\n':
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		case '\\':
			inWord = true
			if i+1 < len(rs) {
				i++
				cur.WriteRune(rs[i])
			}
		case '\'':
			inWord = true
			j := i + 1
			for j < len(rs) && rs[j] != '\'' {
				j++
			}
			if j >= len(rs) {
				return nil, errors.New("unterminated single quote")
			}
			cur.WriteString(string(rs[i+1 : j]))
			i = j
		case '"':
			inWord = true
			j := i + 1
			for ; j < len(rs) && rs[j] != '"'; j++ {
				if rs[j] == '\\' && j+1 < len(rs) && strings.ContainsRune("\"\\$`", rs[j+1]) {
					j++
				}
				cur.WriteRune(rs[j])
			}
			if j >= len(rs) {
				return nil, errors.New("unterminated double quote")
			}
			i = j
		default:
			inWord = true
			cur.WriteRune(r)
		}
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words, nil
}

// command is the single helper that builds a process from an argv.
func command(argv []string) (*exec.Cmd, error) {
	if len(argv) == 0 || argv[0] == "" {
		return nil, errors.New("empty command")
	}
	bin, err := exec.LookPath(argv[0])
	if err != nil {
		return nil, fmt.Errorf("could not find %s in PATH: %w", argv[0], err)
	}
	return exec.Command(bin, argv[1:]...), nil //nolint:gosec // argv is passed as is, never through a shell
}

// ---- browser ----

// BrowserArgv returns the command that opens url: the configured browser
// (EG_BROWSER, the browser config key, BROWSER, resolved by the caller)
// followed by the URL, or the opener of the platform.
func BrowserArgv(configured, url string) ([]string, error) {
	if configured != "" {
		argv, err := SplitWords(configured)
		if err != nil {
			return nil, fmt.Errorf("parse browser command %q: %w", configured, err)
		}
		if len(argv) > 0 {
			return append(argv, url), nil
		}
	}
	switch runtime.GOOS {
	case "darwin":
		return []string{"open", url}, nil
	case "windows":
		return []string{"rundll32", "url.dll,FileProtocolHandler", url}, nil
	}
	return []string{"xdg-open", url}, nil
}

// Browse opens url in the browser and waits for the launcher to return.
func Browse(configured, url string, stdout, stderr io.Writer) error {
	argv, err := BrowserArgv(configured, url)
	if err != nil {
		return err
	}
	cmd, err := command(argv)
	if err != nil {
		return err
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, stdout, stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %s: %w", filepath.Base(argv[0]), err)
	}
	return nil
}

// ---- pager ----

// Pager is a running pager process.
type Pager struct {
	cmd *exec.Cmd
	in  io.WriteCloser
}

// StartPager starts the pager command line with its output on stdout.
// It sets LESS=FRX and LV=-c when unset, as gh does.
func StartPager(commandLine string, stdout, stderr io.Writer) (*Pager, error) {
	argv, err := SplitWords(commandLine)
	if err != nil {
		return nil, fmt.Errorf("parse pager command %q: %w", commandLine, err)
	}
	cmd, err := command(argv)
	if err != nil {
		return nil, err
	}
	env := os.Environ()
	if _, ok := os.LookupEnv("LESS"); !ok {
		env = append(env, "LESS=FRX")
	}
	if _, ok := os.LookupEnv("LV"); !ok {
		env = append(env, "LV=-c")
	}
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = stdout, stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("pager stdin: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start pager: %w", err)
	}
	return &Pager{cmd: cmd, in: in}, nil
}

// Write sends output to the pager; once the user quits the pager the
// rest of the output is dropped silently.
func (p *Pager) Write(b []byte) (int, error) {
	n, err := p.in.Write(b)
	if err != nil {
		return len(b), nil //nolint:nilerr // the pager was closed by the user
	}
	return n, nil
}

// Close closes the pager input and waits for it to exit.
func (p *Pager) Close() error {
	_ = p.in.Close()
	_ = p.cmd.Wait() // the exit status of a pager is not interesting
	return nil
}

// ---- editor ----

// DefaultEditor is used when no editor is configured.
func DefaultEditor() string {
	if runtime.GOOS == "windows" {
		return "notepad"
	}
	return "nano"
}

// Edit writes initial to a temporary file named like fileName, opens it
// in the editor command line and returns the edited content.
func Edit(commandLine, fileName, initial string) (string, error) {
	argv, err := SplitWords(commandLine)
	if err != nil {
		return "", fmt.Errorf("parse editor command %q: %w", commandLine, err)
	}
	dir, err := os.MkdirTemp("", "eg-edit-")
	if err != nil {
		return "", fmt.Errorf("create temporary directory: %w", err)
	}
	defer os.RemoveAll(dir) //nolint:errcheck // best effort cleanup
	path := filepath.Join(dir, fileName)
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	cmd, err := command(append(argv, path))
	if err != nil {
		return "", err
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("run editor %s: %w", filepath.Base(argv[0]), err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(b), nil
}
