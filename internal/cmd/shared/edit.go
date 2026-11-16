package shared

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
)

// EditOptions are the flags of `issue edit` and `pr edit` that work on
// the issue side.
type EditOptions struct {
	Title, Body, BodyFile         string
	AddLabels, RemoveLabels       []string
	AddAssignees, RemoveAssignees []string
	Milestone                     string
	RemoveMilestone               bool
}

// Register adds the flags.
func (o *EditOptions) Register(fl *cmdutil.FlagSet) {
	fl.StringVarP(&o.Title, "title", "t", "", "Set the new title.")
	fl.StringVarP(&o.Body, "body", "b", "", "Set the new body.")
	fl.StringVarP(&o.BodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" to read from standard input)")
	fl.StringArrayVarP(&o.AddAssignees, "add-assignee", "", "Add assigned users by their `login`. Use \"@me\" to assign yourself.")
	fl.StringArrayVarP(&o.RemoveAssignees, "remove-assignee", "", "Remove assigned users by their `login`. Use \"@me\" to unassign yourself.")
	fl.StringArrayVarP(&o.AddLabels, "add-label", "", "Add labels by `name`")
	fl.StringArrayVarP(&o.RemoveLabels, "remove-label", "", "Remove labels by `name`")
	fl.StringVarP(&o.Milestone, "milestone", "m", "", "Edit the milestone by `name`")
	fl.BoolVarP(&o.RemoveMilestone, "remove-milestone", "", false, "Remove the milestone association")
}

// Changed reports whether any of the flags was given.
func (o *EditOptions) Changed(fl *cmdutil.FlagSet) bool {
	for _, n := range []string{"title", "body", "body-file", "add-assignee", "remove-assignee", "add-label",
		"remove-label", "milestone", "remove-milestone"} {
		if fl.Changed(n) {
			return true
		}
	}
	return false
}

// Validate checks flag combinations.
func (o *EditOptions) Validate(fl *cmdutil.FlagSet) error {
	if fl.Changed("body") && fl.Changed("body-file") {
		return cmdutil.FlagErrorf("specify only one of `--body` or `--body-file`")
	}
	if fl.Changed("milestone") && o.RemoveMilestone {
		return cmdutil.FlagErrorf("specify only one of `--milestone` or `--remove-milestone`")
	}
	return nil
}

// Apply sends the changes: title and body with PATCH .../issues/{n},
// or with PATCH .../pulls/{n} together with pullPatch when pullPatch is
// not nil; then the milestone, labels and assignees.
func (o *EditOptions) Apply(f *cmdutil.Factory, c *api.Client, r ghrepo.Repo, n int, fl *cmdutil.FlagSet, pullPatch map[string]any) error {
	patch := map[string]any{}
	text := patch
	if pullPatch != nil {
		text = pullPatch
	}
	if fl.Changed("title") {
		text["title"] = o.Title
	}
	if fl.Changed("body") {
		text["body"] = o.Body
	}
	if fl.Changed("body-file") {
		b, err := ReadBodyFile(f.IO, o.BodyFile)
		if err != nil {
			return err
		}
		text["body"] = b
	}
	if len(pullPatch) > 0 {
		if _, err := c.REST(http.MethodPatch, fmt.Sprintf("%s/pulls/%d", RepoPath(r), n), pullPatch, nil); err != nil {
			return err
		}
	}
	if fl.Changed("milestone") {
		if o.Milestone == "" {
			patch["milestone"] = nil
		} else {
			num, err := MilestoneNumber(c, r, o.Milestone)
			if err != nil {
				return err
			}
			patch["milestone"] = num
		}
	}
	if o.RemoveMilestone {
		patch["milestone"] = nil
	}
	issue := fmt.Sprintf("%s/issues/%d", RepoPath(r), n)
	if len(patch) > 0 {
		if _, err := c.REST(http.MethodPatch, issue, patch, nil); err != nil {
			return err
		}
	}
	if add := SplitList(o.AddLabels); len(add) > 0 {
		if _, err := c.REST(http.MethodPost, issue+"/labels", map[string]any{"labels": add}, nil); err != nil {
			return err
		}
	}
	for _, l := range SplitList(o.RemoveLabels) {
		if _, err := c.REST(http.MethodDelete, issue+"/labels/"+url.PathEscape(l), nil, nil); err != nil && !api.IsNotFound(err) {
			return err
		}
	}
	if len(o.AddAssignees) > 0 {
		logins, err := ReplaceMe(c, o.AddAssignees)
		if err != nil {
			return err
		}
		if _, err := c.REST(http.MethodPost, issue+"/assignees", map[string]any{"assignees": logins}, nil); err != nil {
			return err
		}
	}
	if len(o.RemoveAssignees) > 0 {
		logins, err := ReplaceMe(c, o.RemoveAssignees)
		if err != nil {
			return err
		}
		if _, err := c.REST(http.MethodDelete, issue+"/assignees", map[string]any{"assignees": logins}, nil); err != nil {
			return err
		}
	}
	return nil
}
