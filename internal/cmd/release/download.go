package release

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
)

func newDownload(f *cmdutil.Factory) *cmdutil.Command {
	var (
		repoFlag, dir, archive, output string
		patterns                       []string
		clobber, skipExisting          bool
	)
	c := &cmdutil.Command{
		Use:   "download [<tag>]",
		Short: "Download release assets",
		Long: `Download assets from an ErvisioRepo release.

Without an explicit tag name argument, assets are downloaded from the
latest release in the project. In this case, '--pattern' or '--archive'
is required.`,
		GroupID: "general",
		Example: `# download all assets from a specific release
$ eg release download v1.2.3

# download only Debian packages for the latest release
$ eg release download --pattern '*.deb'

# specify multiple file patterns
$ eg release download -p '*.deb' -p '*.rpm'

# download the archive of the source code for a release
$ eg release download v1.2.3 --archive=zip`,
		Flags: cmdutil.NewFlagSet("download"),
		Args:  cmdutil.MaxArgs(1),
	}
	fl := c.Flags
	addRepoFlag(c, &repoFlag)
	fl.StringArrayVarP(&patterns, "pattern", "p", "Download only assets that match a glob pattern")
	fl.StringVarP(&dir, "dir", "D", ".", "The `directory` to download files into")
	fl.StringVarP(&output, "output", "O", "", "The `file` to write a single asset to (use \"-\" to write to standard output)")
	fl.EnumVarP(&archive, "archive", "A", "", []string{"zip", "tar.gz"}, "Download the source code archive in the specified `format`")
	fl.BoolVarP(&clobber, "clobber", "", false, "Overwrite existing files of the same name")
	fl.BoolVarP(&skipExisting, "skip-existing", "", false, "Skip downloading when files of the same name exist")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		if clobber && skipExisting {
			return cmdutil.FlagErrorf("specify only one of `--clobber` or `--skip-existing`")
		}
		if archive != "" && len(patterns) > 0 {
			return cmdutil.FlagErrorf("specify only one of '--pattern' or '--archive'")
		}
		if len(args) == 0 && archive == "" && len(patterns) == 0 {
			return cmdutil.FlagErrorf("`--pattern` or `--archive` is required when downloading the latest release")
		}
		if output != "" && fl.Changed("dir") {
			return cmdutil.FlagErrorf("specify only one of `--dir` or `--output`")
		}
		r, err := f.BaseRepo(repoFlag)
		if err != nil {
			return err
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		tag := ""
		if len(args) > 0 {
			tag = args[0]
		}
		rel, err := findRelease(client, r, tag)
		if err != nil {
			return err
		}
		type job struct{ url, name, accept, auth string }
		var jobs []job
		if archive != "" {
			src := deref(rel.ZipballURL)
			if archive == "tar.gz" {
				src = deref(rel.TarballURL)
			}
			if src == "" {
				return cmdutil.Unsupported("Downloading source archives", "tarball_url/zipball_url in the release")
			}
			// The archive lives under /api/v1, which takes Bearer tokens.
			jobs = append(jobs, job{url: src, name: r.Name + "-" + rel.TagName + "." + archive, auth: "Bearer " + client.Token})
		} else {
			for _, a := range rel.Assets {
				if len(patterns) > 0 && !matchAny(patterns, a.Name) {
					continue
				}
				jobs = append(jobs, job{url: a.URL, name: a.Name, accept: "application/octet-stream"})
			}
		}
		if len(jobs) == 0 {
			return errors.New("no assets match the file pattern")
		}
		if output != "" && len(jobs) > 1 {
			return errors.New("unable to write more than one asset with `--output`")
		}
		if output == "" {
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return fmt.Errorf("create %s: %w", dir, err)
			}
		}
		for _, j := range jobs {
			h := http.Header{}
			if j.accept != "" {
				h.Set("Accept", j.accept)
			}
			if j.auth != "" && client.Token != "" {
				h.Set("Authorization", j.auth)
			}
			if err := download(f, client, j.url, h, dir, j.name, output, clobber, skipExisting); err != nil {
				return err
			}
		}
		return nil
	}
	return c
}

func matchAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}

func download(f *cmdutil.Factory, c *api.Client, rawURL string, h http.Header, dir, name, output string, clobber, skip bool) error {
	resp, err := c.Request(http.MethodGet, rawURL, nil, h)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body
	if resp.StatusCode >= 300 {
		return api.HandleHTTPError(resp)
	}
	if _, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition")); err == nil && params["filename"] != "" && output == "" {
		name = filepath.Base(params["filename"])
	}
	if output == "-" {
		_, err := io.Copy(f.IO.Out, resp.Body)
		return err
	}
	dest := output
	if dest == "" {
		dest = filepath.Join(dir, filepath.Base(name))
	}
	if _, err := os.Stat(dest); err == nil {
		switch {
		case skip:
			return nil
		case !clobber:
			return fmt.Errorf("%s already exists (use `--clobber` to overwrite file or `--skip-existing` to skip file)", dest)
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".eg-download-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // gone after the rename
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("download %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", dest, err)
	}
	if err := os.Rename(tmp.Name(), dest); err != nil {
		return fmt.Errorf("write %s: %w", dest, err)
	}
	return nil
}
