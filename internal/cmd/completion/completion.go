// Package completion implements `eg completion` and the hidden
// `eg __complete` helper the generated scripts call.
package completion

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
)

// New returns the completion command.
func New(f *cmdutil.Factory) *cmdutil.Command {
	var shell string
	c := &cmdutil.Command{
		Use:   "completion -s <shell>",
		Short: "Generate shell completion scripts",
		Long: `Generate shell completion scripts for Ervisio CLI commands.

When installing Ervisio CLI through a package manager, it's possible that
no additional shell configuration is necessary to gain completion support.

The scripts complete commands, subcommands, flags and your aliases from
the command tree of the running eg (through the hidden "eg __complete").

### bash

First, ensure that you install bash-completion using your package manager.

After, add this to your ~/.bash_profile:

	eval "$(eg completion -s bash)"

### zsh

Generate a _eg completion script and put it somewhere in your $fpath:

	eg completion -s zsh > /usr/local/share/zsh/site-functions/_eg

Ensure that the following is present in your ~/.zshrc:

	autoload -U compinit
	compinit -i

### fish

Generate an eg.fish completion script:

	eg completion -s fish > ~/.config/fish/completions/eg.fish

### PowerShell

Open your profile script with:

	mkdir -Path (Split-Path -Parent $profile) -ErrorAction SilentlyContinue
	notepad $profile

Add the line and save the file:

	Invoke-Expression -Command $(eg completion -s powershell | Out-String)`,
		Flags: cmdutil.NewFlagSet("completion"),
		Args:  cmdutil.NoArgs,
	}
	c.Flags.EnumVarP(&shell, "shell", "s", "", []string{"bash", "zsh", "fish", "powershell"}, "Shell type")
	c.Run = func(*cmdutil.Command, []string) error {
		if shell == "" {
			return cmdutil.FlagErrorf("error: the value for `--shell` is required")
		}
		_, err := io.WriteString(f.IO.Out, scripts[shell])
		return err
	}
	return c
}

var scripts = map[string]string{
	"bash": `# bash completion for eg
_eg() {
    local IFS=$'\n'
    COMPREPLY=( $(eg __complete "${COMP_WORDS[@]:1:COMP_CWORD}" 2>/dev/null) )
}
complete -o default -F _eg eg
`,
	"zsh": `#compdef eg
# zsh completion for eg
_eg() {
    local -a completions
    completions=("${(@f)$(eg __complete "${(@)words[2,$CURRENT]}" 2>/dev/null)}")
    compadd -- ${completions:#}
}
if [ "$funcstack[1]" = "_eg" ]; then
    _eg "$@"
else
    compdef _eg eg
fi
`,
	"fish": `# fish completion for eg
complete -c eg -f -a '(eg __complete (commandline -opc)[2..-1] (commandline -ct) 2>/dev/null)'
`,
	"powershell": `# PowerShell completion for eg
Register-ArgumentCompleter -Native -CommandName 'eg' -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)
    $words = @($commandAst.CommandElements | Select-Object -Skip 1 | ForEach-Object { $_.ToString() })
    if ($wordToComplete -eq '') { $words += '""' }
    & eg __complete @words 2>$null | ForEach-Object {
        [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
    }
}
`,
}

// Complete prints the candidates for the last word of words, walking the
// command tree with the words before it. aliases are offered at the top
// level.
func Complete(w io.Writer, root *cmdutil.Command, aliases []string, words []string) {
	if len(words) == 0 {
		words = []string{""}
	}
	cur := words[len(words)-1]
	if cur == `""` {
		cur = ""
	}
	c := root
	for _, word := range words[:len(words)-1] {
		if strings.HasPrefix(word, "-") {
			continue
		}
		if sub := c.Sub(word); sub != nil {
			c = sub
		}
	}
	var cands []string
	if strings.HasPrefix(cur, "-") {
		if c.Flags != nil {
			cands = c.Flags.FlagNames()
		}
		cands = append(cands, "--help")
	} else if c.Run == nil {
		for _, s := range c.Commands() {
			if !s.Hidden {
				cands = append(cands, s.Name())
			}
		}
		if c == root {
			cands = append(cands, aliases...)
		}
	}
	sort.Strings(cands)
	for _, x := range cands {
		if strings.HasPrefix(x, cur) {
			fmt.Fprintln(w, x)
		}
	}
}
