// Package issue implements `eg issue`.
package issue

import (
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
)

// New returns the issue command group.
func New(f *cmdutil.Factory) *cmdutil.Command {
	c := &cmdutil.Command{
		Use:     "issue <command>",
		Short:   "Manage issues",
		Long:    "Work with ErvisioRepo issues.",
		GroupID: "core",
		Example: `$ eg issue list
$ eg issue create --label bug
$ eg issue view 123 --web`,
	}
	c.AddGroup(cmdutil.Group{ID: "general", Title: "GENERAL COMMANDS"}, cmdutil.Group{ID: "targeted", Title: "TARGETED COMMANDS"})
	c.AddCommand(newCreate(f), newList(f), newStatus(f),
		newClose(f), newComment(f), newDelete(f), newEdit(f), newLock(f, true), newLock(f, false),
		newReopen(f), newView(f))
	return c
}
