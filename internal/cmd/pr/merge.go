package pr

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
)

const enableAutoMerge = `mutation EgEnableAutoMerge($input: EnablePullRequestAutoMergeInput!) {
  enablePullRequestAutoMerge(input: $input) { pullRequest { number } }
}`

const disableAutoMerge = `mutation EgDisableAutoMerge($input: DisablePullRequestAutoMergeInput!) {
  disablePullRequestAutoMerge(input: $input) { pullRequest { number } }
}`

func newMerge(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag, subject, body, bodyFile, matchHead, authorEmail string
		merge, squash, rebase, deleteBranch, auto, disableAuto    bool
		admin                                                     bool
	)
	c := &cmdutil.Command{
		Use:   "merge " + argUse,
		Short: "Merge a pull request",
		Long: `Merge a pull request on ErvisioRepo.

Without an argument, the pull request that belongs to the current branch
is selected.

When targeting a branch that requires a merge queue, no merge strategy is
required; merge queues are not supported by ErvisioRepo.`,
		GroupID: "targeted",
		Flags:   cmdutil.NewFlagSet("merge"),
		Args:    selectorArgs,
	}
	fl := c.Flags
	shared.AddRepoFlag(c, &repoFlag)
	fl.BoolVarP(&admin, "admin", "", false, "Use administrator privileges to merge a pull request that does not meet requirements (not supported)")
	fl.StringVarP(&authorEmail, "author-email", "A", "", "Email `text` for merge commit author (not supported)")
	fl.BoolVarP(&auto, "auto", "", false, "Automatically merge only after necessary requirements are met")
	fl.StringVarP(&body, "body", "b", "", "Body `text` for the merge commit")
	fl.StringVarP(&bodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" to read from standard input)")
	fl.BoolVarP(&deleteBranch, "delete-branch", "d", false, "Delete the local and remote branch after merge")
	fl.BoolVarP(&disableAuto, "disable-auto", "", false, "Disable auto-merge for this pull request")
	fl.StringVarP(&matchHead, "match-head-commit", "", "", "Commit `SHA` that the pull request head must match to allow merge")
	fl.BoolVarP(&merge, "merge", "m", false, "Merge the commits with the base branch")
	fl.BoolVarP(&rebase, "rebase", "r", false, "Rebase the commits onto the base branch")
	fl.BoolVarP(&squash, "squash", "s", false, "Squash the commits into one commit and merge it into the base branch")
	fl.StringVarP(&subject, "subject", "t", "", "Subject `text` for the merge commit")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		if admin {
			return cmdutil.Unsupported("Merging with administrator privileges (--admin)", "a bypass option in PUT /repos/{owner}/{repo}/pulls/{number}/merge")
		}
		if authorEmail != "" {
			return cmdutil.Unsupported("Setting the merge commit author (--author-email)", "authorEmail in mergePullRequest")
		}
		methods := 0
		method := ""
		for _, m := range []struct {
			on   bool
			name string
		}{{merge, "merge"}, {squash, "squash"}, {rebase, "rebase"}} {
			if m.on {
				methods++
				method = m.name
			}
		}
		if methods > 1 {
			return cmdutil.FlagErrorf("only one of --merge, --rebase, or --squash can be enabled")
		}
		if fl.Changed("body") && bodyFile != "" {
			return cmdutil.FlagErrorf("specify only one of `--body` or `--body-file`")
		}
		ios := f.IO
		if method == "" && !disableAuto && !ios.CanPrompt() {
			return cmdutil.FlagErrorf("--merge, --rebase, or --squash required when not running interactively")
		}
		if bodyFile != "" {
			b, err := shared.ReadBodyFile(ios, bodyFile)
			if err != nil {
				return err
			}
			body = b
			fl.Set("body", b) //nolint:errcheck // known flag
		}
		fd, p, err := find(f, repoFlag, args)
		if err != nil {
			return err
		}
		cs := ios.ColorScheme()
		if disableAuto {
			in := map[string]any{"pullRequestId": p.NodeID}
			if err := shared.GraphQLMutation(fd.client, "disablePullRequestAutoMerge", disableAutoMerge, map[string]any{"input": in}, nil); err != nil {
				return err
			}
			if ios.IsStderrTTY() {
				fmt.Fprintf(ios.ErrOut, "%s Auto-merge disabled for pull request %s#%d\n", cs.SuccessIcon(), fd.repo.FullName(), p.Number)
			}
			return nil
		}
		if p.StateName() == "MERGED" {
			fmt.Fprintf(ios.ErrOut, "%s Pull request %s was already merged\n", cs.WarningIcon(), prRef(fd.repo, p))
			if !deleteBranch {
				return nil
			}
			return deleteBranches(f, fd, p, false)
		}
		if p.State == "closed" {
			return fmt.Errorf("pull request %s is closed and cannot be merged", prRef(fd.repo, p))
		}
		if method == "" {
			pr := &cmdutil.Prompter{IO: ios}
			options := []string{"Create a merge commit", "Rebase and merge", "Squash and merge"}
			i, err := pr.Select("What merge method would you like to use?", 0, options)
			if err != nil {
				return err
			}
			method = []string{"merge", "rebase", "squash"}[i]
			if !fl.Changed("delete-branch") && !p.IsCrossRepository() {
				if deleteBranch, err = pr.Confirm("Delete the branch locally and on ErvisioRepo?", false); err != nil {
					return err
				}
			}
			ok, err := pr.Confirm("Submit?", true)
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.ErrCancel
			}
		}
		if auto {
			in := map[string]any{"pullRequestId": p.NodeID, "mergeMethod": strings.ToUpper(method)}
			if fl.Changed("subject") {
				in["commitHeadline"] = subject
			}
			if fl.Changed("body") {
				in["commitBody"] = body
			}
			if err := shared.GraphQLMutation(fd.client, "enablePullRequestAutoMerge", enableAutoMerge, map[string]any{"input": in}, nil); err != nil {
				return err
			}
			if ios.IsStderrTTY() {
				fmt.Fprintf(ios.ErrOut, "%s Pull request %s#%d will be automatically merged via %s when all requirements are met\n",
					cs.SuccessIcon(), fd.repo.FullName(), p.Number, method)
			}
			return nil
		}
		req := map[string]any{"merge_method": method}
		if fl.Changed("subject") {
			req["commit_title"] = subject
		}
		if fl.Changed("body") {
			req["commit_message"] = body
		}
		if matchHead != "" {
			req["sha"] = matchHead
		}
		if _, err := fd.client.REST(http.MethodPut, fmt.Sprintf("%s/pulls/%d/merge", shared.RepoPath(fd.repo), p.Number), req, nil); err != nil {
			var he *api.HTTPError
			if errors.As(err, &he) && (he.StatusCode == http.StatusMethodNotAllowed || he.StatusCode == http.StatusConflict) {
				return fmt.Errorf("Pull request %s is not mergeable: %s", prRef(fd.repo, p), he.Message) //nolint:staticcheck // gh wording
			}
			return err
		}
		if ios.IsStderrTTY() {
			verb := map[string]string{"merge": "Merged", "rebase": "Rebased and merged", "squash": "Squashed and merged"}[method]
			fmt.Fprintf(ios.ErrOut, "%s %s pull request %s\n", cs.Magenta("✓"), verb, prRef(fd.repo, p))
		}
		if deleteBranch {
			return deleteBranches(f, fd, p, true)
		}
		return nil
	}
	return c
}

// deleteBranches removes the local head branch (switching to the base
// branch when it is checked out) and the remote one of a same-repository
// pull request.
func deleteBranches(f *cmdutil.Factory, fd *finder, p *shared.PullRequest, justMerged bool) error {
	ios := f.IO
	cs := ios.ColorScheme()
	g := f.Git("")
	if _, err := g.GitDir(); err == nil && hasLocalBranch(f, p.Head.Ref) {
		current, _ := g.CurrentBranch()
		switched := ""
		if current == p.Head.Ref {
			target := p.Base.Ref
			if !hasLocalBranch(f, target) {
				return fmt.Errorf("cannot delete the checked out branch %s: local branch %s does not exist", p.Head.Ref, target)
			}
			if err := g.Run("checkout", target); err != nil {
				return err
			}
			if justMerged {
				_ = g.Run("pull", "--ff-only") // best effort, as gh
			}
			switched = target
		}
		if err := g.Run("branch", "-D", p.Head.Ref); err != nil {
			return err
		}
		if ios.IsStderrTTY() {
			msg := "Deleted local branch " + cs.Cyan(p.Head.Ref)
			if switched != "" {
				msg += " and switched to branch " + cs.Cyan(switched)
			}
			fmt.Fprintf(ios.ErrOut, "%s %s\n", cs.Red("✓"), msg)
		}
	}
	if p.IsCrossRepository() {
		return nil
	}
	ref := fmt.Sprintf("%s/git/refs/heads/%s", shared.RepoPath(fd.repo), escapeRef(p.Head.Ref))
	if _, err := fd.client.REST(http.MethodDelete, ref, nil, nil); err != nil {
		if api.IsStatus(err, http.StatusUnprocessableEntity) || api.IsNotFound(err) {
			return nil // already deleted
		}
		return fmt.Errorf("failed to delete remote branch %s: %w", p.Head.Ref, err)
	}
	if ios.IsStderrTTY() {
		fmt.Fprintf(ios.ErrOut, "%s Deleted remote branch %s\n", cs.Red("✓"), cs.Cyan(p.Head.Ref))
	}
	return nil
}

// escapeRef escapes each segment of a branch name.
func escapeRef(ref string) string {
	parts := strings.Split(ref, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}
