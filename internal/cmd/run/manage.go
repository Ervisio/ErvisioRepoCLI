package run

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
)

func newWatch(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag            string
		interval            int
		exitStatus, compact bool
	)
	c := &cmdutil.Command{
		Use:   "watch <run-id>",
		Short: "Watch a run until it completes, showing its progress",
		Long: `Watch a run until it completes, showing its progress.

By default, all steps are displayed. The --compact option can be used to only
show the relevant/failed steps.`,
		Example: `# Watch a run until it's done
$ eg run watch

# Run some other command when the run is finished
$ eg run watch && notify-send 'run is done!'`,
		Flags: cmdutil.NewFlagSet("watch"),
		Args:  cmdutil.MaxArgs(1),
	}
	fl := c.Flags
	shared.AddRepoFlag(c, &repoFlag)
	fl.BoolVarP(&exitStatus, "exit-status", "", false, "Exit with non-zero status if run fails")
	fl.IntVarP(&interval, "interval", "i", 3, "Refresh interval in seconds")
	fl.BoolVarP(&compact, "compact", "", false, "Show only relevant/failed steps")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		r, err := f.BaseRepo(repoFlag)
		if err != nil {
			return err
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		var runID int64
		if len(args) > 0 {
			if runID, err = parseRunID(args[0]); err != nil {
				return err
			}
		} else if runID, err = pickRun(f, client, r, func(run *Run) bool { return !run.Completed() && !run.External },
			"found no in progress runs to watch"); err != nil {
			return err
		}
		ios := f.IO
		cs := ios.ColorScheme()
		run, err := FetchRun(client, r, runID, 0)
		if err != nil {
			return err
		}
		if run.Completed() {
			fmt.Fprintf(ios.Out, "Run %s (%s) has already completed with '%s'\n", cs.Bold(run.Title()), cs.Cyan(strconv.FormatInt(run.ID, 10)), run.Conc())
			if exitStatus && run.Failed() {
				return cmdutil.ErrSilent
			}
			return nil
		}
		if run.External {
			return externalNote(run, "jobs")
		}
		for {
			jobs, err := FetchJobs(client, r, run.ID, 0)
			if err != nil {
				return err
			}
			if ios.IsStdoutTTY() {
				fmt.Fprint(ios.Out, "\x1b[H\x1b[2J")
				fmt.Fprintf(ios.Out, "Refreshing run status every %d seconds. Press Ctrl+C to quit.\n\n", interval)
			}
			printRun(ios, run, jobs, nil, !compact, false)
			if run.Completed() {
				break
			}
			time.Sleep(time.Duration(max(interval, 1)) * time.Second)
			if run, err = FetchRun(client, r, runID, 0); err != nil {
				return err
			}
		}
		symbol := Symbol(cs, run.Status, run.Conc())
		fmt.Fprintf(ios.Out, "\n%s Run %s (%s) completed with '%s'\n", symbol, cs.Bold(run.Title()), cs.Cyan(strconv.FormatInt(run.ID, 10)), run.Conc())
		if exitStatus && run.Failed() {
			return cmdutil.ErrSilent
		}
		return nil
	}
	return c
}

func newRerun(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag, job string
		failed, debug bool
	)
	c := &cmdutil.Command{
		Use:   "rerun [<run-id>]",
		Short: "Rerun a run",
		Long: `Rerun an entire run.

This ErvisioRepo server re-runs whole runs only: --failed and --job are
not available. Runs imported from GitHub are re-run on GitHub.`,
		Flags: cmdutil.NewFlagSet("rerun"),
		Args:  cmdutil.MaxArgs(1),
	}
	fl := c.Flags
	shared.AddRepoFlag(c, &repoFlag)
	fl.BoolVarP(&failed, "failed", "", false, "Rerun only failed jobs, including dependencies (not supported)")
	fl.StringVarP(&job, "job", "j", "", "Rerun a specific job ID from a run, including dependencies (not supported)")
	fl.BoolVarP(&debug, "debug", "d", false, "Rerun with debug logging (not supported)")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		switch {
		case failed:
			return errors.New("re-running only the failed jobs is not supported by this ErvisioRepo server " +
				"(POST .../rerun-failed-jobs answers 422): run `eg run rerun <run-id>` to re-run the whole run")
		case job != "":
			return cmdutil.Unsupported("Re-running a single job", "POST /repos/{owner}/{repo}/actions/jobs/{id}/rerun")
		case debug:
			return cmdutil.Unsupported("Re-running with debug logging", "enable_debug_logging in POST .../rerun")
		}
		r, err := f.BaseRepo(repoFlag)
		if err != nil {
			return err
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		var runID int64
		if len(args) > 0 {
			if runID, err = parseRunID(args[0]); err != nil {
				return err
			}
		} else if runID, err = pickRun(f, client, r, func(run *Run) bool { return run.Completed() && !run.External },
			"found no completed runs to rerun"); err != nil {
			return err
		}
		run, err := FetchRun(client, r, runID, 0)
		if err != nil {
			return err
		}
		if run.External {
			return fmt.Errorf("run %d was imported from GitHub; re-run it there: %s", run.ID, run.HTMLURL)
		}
		if _, err := client.REST(http.MethodPost, ActionsPath(r)+"/runs/"+strconv.FormatInt(runID, 10)+"/rerun", nil, nil); err != nil {
			if api.IsStatus(err, http.StatusConflict) {
				return fmt.Errorf("run %d cannot be rerun; its workflow file may be broken or it is still running", runID)
			}
			return err
		}
		if f.IO.IsStdoutTTY() {
			fmt.Fprintf(f.IO.Out, "%s Requested rerun of run %s\n", f.IO.ColorScheme().SuccessIcon(), f.IO.ColorScheme().Cyan(strconv.FormatInt(runID, 10)))
		}
		return nil
	}
	return c
}

func newCancel(f *cmdutil.Factory) *cmdutil.Command {
	var repoFlag string
	c := &cmdutil.Command{
		Use:   "cancel [<run-id>]",
		Short: "Cancel a workflow run",
		Flags: cmdutil.NewFlagSet("cancel"),
		Args:  cmdutil.MaxArgs(1),
	}
	shared.AddRepoFlag(c, &repoFlag)
	c.Run = func(_ *cmdutil.Command, args []string) error {
		r, err := f.BaseRepo(repoFlag)
		if err != nil {
			return err
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		var runID int64
		if len(args) > 0 {
			if runID, err = parseRunID(args[0]); err != nil {
				return err
			}
		} else if runID, err = pickRun(f, client, r, func(run *Run) bool { return !run.Completed() && !run.External },
			"found no in progress runs to cancel"); err != nil {
			return err
		}
		if _, err := client.REST(http.MethodPost, ActionsPath(r)+"/runs/"+strconv.FormatInt(runID, 10)+"/cancel", nil, nil); err != nil {
			switch {
			case api.IsNotFound(err):
				return fmt.Errorf("could not find any workflow run with ID %d", runID)
			case api.IsStatus(err, http.StatusConflict):
				return errors.New("Cannot cancel a workflow run that is completed") //nolint:staticcheck // gh wording
			}
			return err
		}
		if f.IO.IsStdoutTTY() {
			fmt.Fprintf(f.IO.Out, "%s Request to cancel workflow %d submitted.\n", f.IO.ColorScheme().SuccessIcon(), runID)
		}
		return nil
	}
	return c
}

func newDelete(*cmdutil.Factory) *cmdutil.Command {
	var repoFlag string
	c := &cmdutil.Command{
		Use:   "delete [<run-id>]",
		Short: "Delete a workflow run",
		Long:  "Delete a workflow run. Not supported: runs cannot be deleted on an ErvisioRepo server.",
		Flags: cmdutil.NewFlagSet("delete"),
		Args:  cmdutil.MaxArgs(1),
	}
	shared.AddRepoFlag(c, &repoFlag)
	c.Run = func(*cmdutil.Command, []string) error {
		return cmdutil.Unsupported("Deleting workflow runs", "DELETE /repos/{owner}/{repo}/actions/runs/{id}")
	}
	return c
}

func newDownload(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag, dir   string
		names, patterns []string
	)
	c := &cmdutil.Command{
		Use:   "download [<run-id>]",
		Short: "Download artifacts generated by a workflow run",
		Long: `Download artifacts generated by an Ervisio Actions workflow run.

The contents of each artifact will be extracted under separate directories
based on the artifact name. If only a single artifact is specified, it will
be extracted into the current directory.

Without a run ID, artifacts of every run of the repository are candidates;
the newest artifact of each name is downloaded.`,
		Example: `# Download all artifacts generated by a workflow run
$ eg run download <run-id>

# Download a specific artifact within a run
$ eg run download <run-id> -n <name>

# Download specific artifacts across all runs in a repository
$ eg run download -n <name1> -n <name2>`,
		Flags: cmdutil.NewFlagSet("download"),
		Args:  cmdutil.MaxArgs(1),
	}
	fl := c.Flags
	shared.AddRepoFlag(c, &repoFlag)
	fl.StringVarP(&dir, "dir", "D", ".", "The directory to download artifacts into")
	fl.StringArrayVarP(&names, "name", "n", "Download artifacts that match any of the given names")
	fl.StringArrayVarP(&patterns, "pattern", "p", "Download artifacts that match a glob pattern")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		r, err := f.BaseRepo(repoFlag)
		if err != nil {
			return err
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		var runID int64
		if len(args) > 0 {
			if runID, err = parseRunID(args[0]); err != nil {
				return err
			}
			run, err := FetchRun(client, r, runID, 0)
			if err != nil {
				return err
			}
			if run.External {
				return externalNote(run, "artifacts")
			}
		}
		all, err := listArtifacts(client, r, runID)
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		var picked []Artifact
		for _, a := range all {
			if a.Expired || seen[a.Name] {
				continue
			}
			if (len(names) > 0 || len(patterns) > 0) && !matchArtifact(a.Name, names, patterns) {
				continue
			}
			seen[a.Name] = true
			picked = append(picked, a)
		}
		if len(picked) == 0 {
			return errors.New("no valid artifacts found to download")
		}
		flat := len(names) == 1 && len(patterns) == 0
		for _, a := range picked {
			dest := dir
			if !flat {
				dest = filepath.Join(dir, filepath.Base(a.Name))
			}
			if err := downloadArtifact(client, a, dest); err != nil {
				return fmt.Errorf("error downloading %s: %w", a.Name, err)
			}
		}
		return nil
	}
	return c
}

func matchArtifact(name string, names, patterns []string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	for _, p := range patterns {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}

// downloadArtifact fetches the zip of a (following the signed redirect)
// and extracts it into dest without overwriting files.
func downloadArtifact(c *api.Client, a Artifact, dest string) error {
	resp, err := c.Request(http.MethodGet, a.ArchiveDownloadURL, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body
	if resp.StatusCode >= 300 {
		return api.HandleHTTPError(resp)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read archive: %w", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	if err := os.MkdirAll(dest, 0o750); err != nil {
		return fmt.Errorf("create %s: %w", dest, err)
	}
	root, err := os.OpenRoot(dest)
	if err != nil {
		return fmt.Errorf("open %s: %w", dest, err)
	}
	defer root.Close() //nolint:errcheck // directory handle
	for _, zf := range zr.File {
		name := path.Clean(strings.ReplaceAll(zf.Name, "\\", "/"))
		if strings.HasPrefix(name, "../") || name == ".." || path.IsAbs(name) {
			return fmt.Errorf("unsafe path in archive: %s", zf.Name)
		}
		if zf.FileInfo().IsDir() {
			if err := root.MkdirAll(name, 0o750); err != nil {
				return fmt.Errorf("create %s: %w", name, err)
			}
			continue
		}
		if d := path.Dir(name); d != "." {
			if err := root.MkdirAll(d, 0o750); err != nil {
				return fmt.Errorf("create %s: %w", d, err)
			}
		}
		if err := extract(root, zf, name); err != nil {
			return err
		}
	}
	return nil
}

func extract(root *os.Root, zf *zip.File, name string) error {
	rc, err := zf.Open()
	if err != nil {
		return fmt.Errorf("open %s: %w", zf.Name, err)
	}
	defer rc.Close() //nolint:errcheck // read-only
	mode := os.FileMode(0o644)
	if zf.Mode()&0o111 != 0 {
		mode = 0o755
	}
	out, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return fmt.Errorf("would not overwrite %s: %w", name, err)
	}
	if _, err := io.Copy(out, rc); err != nil { //nolint:gosec // artifacts are the user's own files
		_ = out.Close()
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}
