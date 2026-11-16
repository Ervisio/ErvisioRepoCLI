package release

import (
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Ervisio/ErvisioRepoCLI/internal/api"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmd/shared"
	"github.com/Ervisio/ErvisioRepoCLI/internal/cmdutil"
	"github.com/Ervisio/ErvisioRepoCLI/internal/ghrepo"
)

// uploadFile is a file to attach, with an optional "#label".
type uploadFile struct {
	path, name, label string
	size              int64
}

func parseFiles(args []string) ([]uploadFile, error) {
	var out []uploadFile
	for _, a := range args {
		p, label, _ := strings.Cut(a, "#")
		fi, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("could not access %s: %w", p, err)
		}
		if fi.IsDir() {
			return nil, fmt.Errorf("%s is a directory", p)
		}
		out = append(out, uploadFile{path: p, name: filepath.Base(p), label: label, size: fi.Size()})
	}
	return out, nil
}

// uploadAssets sends files to the upload_url of rel; with clobber, an
// asset of the same name is replaced.
func uploadAssets(c *api.Client, r ghrepo.Repo, rel *Release, files []uploadFile, clobber bool) error {
	existing := map[string]int64{}
	for _, a := range rel.Assets {
		existing[a.Name] = a.ID
	}
	var dup []string
	for _, fu := range files {
		if _, ok := existing[fu.name]; ok && !clobber {
			dup = append(dup, fu.name)
		}
	}
	if len(dup) > 0 {
		return fmt.Errorf("asset under the same name already exists: %v", dup)
	}
	base := rel.UploadURL
	if i := strings.Index(base, "{"); i >= 0 {
		base = base[:i]
	}
	if base == "" {
		base = c.BaseURL() + "/api/uploads/" + shared.RepoPath(r) + fmt.Sprintf("/releases/%d/assets", rel.ID)
	}
	for _, fu := range files {
		if id, ok := existing[fu.name]; ok {
			if _, err := c.REST(http.MethodDelete, fmt.Sprintf("%s/releases/assets/%d", shared.RepoPath(r), id), nil, nil); err != nil {
				return fmt.Errorf("delete existing asset %s: %w", fu.name, err)
			}
		}
		if err := uploadOne(c, base, fu); err != nil {
			return err
		}
	}
	return nil
}

func uploadOne(c *api.Client, base string, fu uploadFile) error {
	fh, err := os.Open(fu.path)
	if err != nil {
		return fmt.Errorf("open %s: %w", fu.path, err)
	}
	defer fh.Close() //nolint:errcheck // read-only file
	q := url.Values{"name": {fu.name}}
	if fu.label != "" {
		q.Set("label", fu.label)
	}
	ct := mime.TypeByExtension(filepath.Ext(fu.name))
	if ct == "" {
		ct = "application/octet-stream"
	}
	if err := c.Upload(base+"?"+q.Encode(), fh, fu.size, ct, nil); err != nil {
		if api.IsNotFound(err) {
			return cmdutil.Unsupported("Uploading release assets", "POST /api/uploads/repos/{owner}/{repo}/releases/{id}/assets")
		}
		return fmt.Errorf("upload %s: %w", fu.name, err)
	}
	return nil
}

func newUpload(f *cmdutil.Factory) *cmdutil.Command {
	var repoFlag string
	var clobber bool
	c := &cmdutil.Command{
		Use:   "upload <tag> <files>...",
		Short: "Upload assets to a release",
		Long: `Upload asset files to an ErvisioRepo release.

To define a display label for an asset, append text starting with '#'
after the file name.`,
		GroupID: "general",
		Flags:   cmdutil.NewFlagSet("upload"),
		Args: func(a []string) error {
			if len(a) < 2 {
				return errors.New("requires at least 2 arg(s), only received " + fmt.Sprint(len(a)))
			}
			return nil
		},
	}
	addRepoFlag(c, &repoFlag)
	c.Flags.BoolVarP(&clobber, "clobber", "", false, "Overwrite existing assets of the same name")
	c.Run = func(_ *cmdutil.Command, args []string) error {
		files, err := parseFiles(args[1:])
		if err != nil {
			return err
		}
		r, err := f.BaseRepo(repoFlag)
		if err != nil {
			return err
		}
		client, err := f.Client(r.Host)
		if err != nil {
			return err
		}
		rel, err := findRelease(client, r, args[0])
		if err != nil {
			return err
		}
		if err := uploadAssets(client, r, rel, files, clobber); err != nil {
			return err
		}
		if f.IO.IsStdoutTTY() {
			fmt.Fprintf(f.IO.ErrOut, "Successfully uploaded %d asset(s) to %s\n", len(files), rel.TagName)
		}
		return nil
	}
	return c
}
