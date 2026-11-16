// Package alias implements `eg alias` and the expansion of aliases.
package alias

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/config"
	"github.com/Ervisio/ErvisioRepoCLI/internal/launch"
)

// New returns the alias command group.
func New(f *cmdutil.Factory) *cmdutil.Command {
	c := &cmdutil.Command{
		Use:   "alias <command>",
		Short: "Create command shortcuts",
		Long: `Aliases can be used to make shortcuts for eg commands or to compose
multiple commands.

Run eg help alias set to learn more.`,
	}
	c.AddCommand(newSet(f, c), newList(f), newDelete(f))
	return c
}

var errShell = errors.New("shell aliases are not supported by eg: it never runs commands through a shell")

func newSet(f *cmdutil.Factory, group *cmdutil.Command) *cmdutil.Command {
	var shell, clobber bool
	c := &cmdutil.Command{
		Use:   "set <alias> <expansion>",
		Short: "Create a shortcut for an eg command",
		Long: `Define a word that will expand to a full eg command when invoked.

The expansion may specify additional arguments and flags. If the expansion
includes positional placeholders such as "$1", extra arguments that follow
the alias will be inserted appropriately. Otherwise, extra arguments will
be appended to the expanded command.

Use "-" as expansion argument to read the expansion string from standard
input. This is useful to avoid quoting issues when defining expansions.

Shell aliases (--shell, or an expansion starting with "!") are not
supported: eg never runs commands through a shell.`,
		Example: `# note: Command Prompt on Windows requires using double quotes for arguments
$ eg alias set pv 'pr view'
$ eg pv -w 123  #=> eg pr view -w 123

$ eg alias set bugs 'issue list --label=bugs'
$ eg bugs

$ eg alias set homework 'issue list --assignee @me'
$ eg homework

$ eg alias set epicsBy 'issue list --author="$1" --label="epic"'
$ eg epicsBy vilmibm  #=> eg issue list --author="vilmibm" --label="epic"`,
		Flags: cmdutil.NewFlagSet("set"),
		Args:  cmdutil.ExactArgs(2, "cannot create alias: alias and expansion arguments required"),
	}
	c.Flags.BoolVarP(&shell, "shell", "s", false, "Declare an alias to be passed through a shell interpreter (not supported)")
	c.Flags.BoolVarP(&clobber, "clobber", "", false, "Overwrite existing aliases of the same name")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		if shell {
			return errShell
		}
		name, expansion := args[0], args[1]
		if expansion == "-" {
			data, err := io.ReadAll(f.IO.In)
			if err != nil {
				return fmt.Errorf("failed to read expansion from standard input: %w", err)
			}
			expansion = strings.TrimSpace(string(data))
		}
		if strings.HasPrefix(expansion, "!") {
			return errShell
		}
		cfg, err := f.Config()
		if err != nil {
			return err
		}
		root := group.Root()
		if root.Sub(name) != nil {
			return fmt.Errorf("could not create alias %s: already a eg command or alias", name)
		}
		words, err := launch.SplitWords(expansion)
		if err != nil || len(words) == 0 {
			return fmt.Errorf("could not create alias %s: invalid expansion %q", name, expansion)
		}
		_, existing := cfg.Aliases()
		if root.Sub(words[0]) == nil {
			if _, ok := existing[words[0]]; !ok {
				return fmt.Errorf("could not create alias %s: expansion does not correspond to a eg command, extension, or alias", name)
			}
		}
		cs := f.IO.ColorScheme()
		old, had := existing[name]
		if had && !clobber {
			return fmt.Errorf("could not create alias %s: name already taken; use --clobber to overwrite", name)
		}
		if err := cfg.SetAlias(name, expansion); err != nil {
			return err
		}
		if f.IO.IsStdoutTTY() {
			if had {
				fmt.Fprintf(f.IO.Out, "%s Changed alias %s from %s to %s\n", cs.Yellow("!"), cs.Bold(name), cs.Bold(old), cs.Bold(expansion))
			} else {
				fmt.Fprintf(f.IO.Out, "%s Added alias %s\n", cs.SuccessIcon(), cs.Bold(name))
			}
		}
		return nil
	}
	return c
}

func newList(f *cmdutil.Factory) *cmdutil.Command {
	return &cmdutil.Command{
		Use:     "list",
		Short:   "List your aliases",
		Long:    "This command prints out all of the aliases eg is configured to use.",
		Aliases: []string{"ls"},
		Args:    cmdutil.NoArgs,
		Run: func(*cmdutil.Command, []string) error {
			cfg, err := f.Config()
			if err != nil {
				return err
			}
			names, all := cfg.Aliases()
			if len(names) == 0 {
				if f.IO.IsStdoutTTY() {
					fmt.Fprintln(f.IO.ErrOut, "no aliases configured")
				}
				return cmdutil.ErrSilent
			}
			sort.Strings(names)
			for _, n := range names {
				fmt.Fprintf(f.IO.Out, "%s: %s\n", n, quoteIfNeeded(all[n]))
			}
			return nil
		},
	}
}

var plainRE = regexp.MustCompile(`^[\w./@=$ -]+$`)

func quoteIfNeeded(s string) string {
	if plainRE.MatchString(s) {
		return s
	}
	return strconv.Quote(s)
}

func newDelete(f *cmdutil.Factory) *cmdutil.Command {
	var all bool
	c := &cmdutil.Command{
		Use:   "delete {<alias> | --all}",
		Short: "Delete set aliases",
		Flags: cmdutil.NewFlagSet("delete"),
		Args:  cmdutil.MaxArgs(1),
	}
	c.Flags.BoolVarP(&all, "all", "", false, "Delete all aliases")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		if all == (len(args) == 1) {
			return cmdutil.FlagErrorf("specify an alias to delete or `--all`")
		}
		cfg, err := f.Config()
		if err != nil {
			return err
		}
		names, existing := cfg.Aliases()
		if !all {
			if _, ok := existing[args[0]]; !ok {
				return fmt.Errorf("no such alias %s", args[0])
			}
			names = args
		} else if len(names) == 0 {
			return errors.New("no aliases configured")
		}
		cs := f.IO.ColorScheme()
		for _, n := range names {
			if err := cfg.DeleteAlias(n); err != nil {
				return err
			}
			if f.IO.IsStdoutTTY() {
				fmt.Fprintf(f.IO.Out, "%s Deleted alias %s; was %s\n", cs.Red("✓"), n, existing[n])
			}
		}
		return nil
	}
	return c
}

var placeholderRE = regexp.MustCompile(`\$\d+`)

// Expand rewrites args when args[0] is an alias of cfg (and not a
// command of root). It reports whether an alias was used.
func Expand(cfg *config.Config, root *cmdutil.Command, args []string) ([]string, bool, error) {
	if len(args) == 0 || root.Sub(args[0]) != nil {
		return args, false, nil
	}
	_, all := cfg.Aliases()
	seen := map[string]bool{}
	expanded := false
	for len(args) > 0 && root.Sub(args[0]) == nil {
		exp, ok := all[args[0]]
		if !ok {
			break
		}
		if seen[args[0]] {
			return nil, false, fmt.Errorf("alias %s expands to itself", args[0])
		}
		seen[args[0]] = true
		if strings.HasPrefix(exp, "!") {
			return nil, false, errShell
		}
		next, err := expandOne(exp, args[1:])
		if err != nil {
			return nil, false, fmt.Errorf("alias %s: %w", args[0], err)
		}
		args, expanded = next, true
	}
	return args, expanded, nil
}

// expandOne splits exp into words, fills their $N placeholders with
// extra and appends the arguments that were not used.
func expandOne(exp string, extra []string) ([]string, error) {
	words, err := launch.SplitWords(exp)
	if err != nil {
		return nil, fmt.Errorf("invalid expansion: %w", err)
	}
	used := map[int]bool{}
	for i, w := range words {
		words[i] = placeholderRE.ReplaceAllStringFunc(w, func(m string) string {
			n, _ := strconv.Atoi(m[1:])
			if n < 1 || n > len(extra) {
				err = fmt.Errorf("not enough arguments for %s", m)
				return m
			}
			used[n-1] = true
			return extra[n-1]
		})
	}
	if err != nil {
		return nil, err
	}
	for i, a := range extra {
		if !used[i] {
			words = append(words, a)
		}
	}
	return words, nil
}
