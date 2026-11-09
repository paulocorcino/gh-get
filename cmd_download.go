package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	destpolicy "github.com/paulocorcino/gh-get/internal/dest"
	"github.com/paulocorcino/gh-get/internal/fetch"
	"github.com/paulocorcino/gh-get/internal/ghurl"
	"github.com/paulocorcino/gh-get/internal/meta"
)

// downloadRequest is one download asked for on the command line.
type downloadRequest struct {
	URL   string
	Dest  string // "" names the destination after the folder (or the repo)
	Force bool
	Token string
	Ref   string // overrides the ref in the URL when set
}

func runDownload(req downloadRequest, stdout, stderr io.Writer) error {
	warnf := warnTo(stderr)
	client := fetch.New(req.Token, warnf)

	src, err := resolveDownload(client, req.URL, req.Ref)
	if err != nil {
		return err
	}
	dest := req.Dest
	if dest == "" {
		dest = "./" + defaultDownloadName(src)
	}
	plan, err := planDownload(dest, req.Force)
	if err != nil {
		return err
	}
	var mode fetch.WriteMode
	switch plan.Action {
	case destpolicy.Refuse:
		return errors.New(plan.Reason) //nolint:err113 // the destination policy owns this message; callers only print it
	case destpolicy.WriteInPlace:
		mode = fetch.Merge
	case destpolicy.Replace:
		mode = fetch.Replace
	}

	printDownloadHeader(stdout, src, dest, plan.Action == destpolicy.WriteInPlace)
	files, err := client.Materialize(src, dest, mode, nil, warnf)
	if err != nil {
		return fmt.Errorf("downloading: %w", err)
	}
	m := metaFrom(src)
	m.Files = files
	if err := meta.Write(dest, m); err != nil {
		return fmt.Errorf("recording the source: %w", err)
	}

	outln(stdout, "Done:")
	outln(stdout, "  "+dest)
	return nil
}

// resolveDownload resolves a GitHub URL to the source to download. A non-empty
// refOverride (--ref) takes precedence over the branch embedded in the URL; the
// URL's ref is still resolved so the folder path can be split correctly.
func resolveDownload(client *fetch.Client, url, refOverride string) (ghurl.Source, error) {
	partial, err := ghurl.Parse(url)
	if err != nil {
		return ghurl.Source{}, fmt.Errorf("parsing URL: %w", err)
	}
	src, err := partial.Resolve(
		client.CheckRef(partial.Owner, partial.Repo),
		client.ListRefs(partial.Owner, partial.Repo),
		func() (string, error) { return client.DefaultBranch(partial.Owner, partial.Repo) },
	)
	if err != nil {
		return ghurl.Source{}, fmt.Errorf("resolving URL: %w", err)
	}
	if refOverride == "" {
		return src, nil
	}
	sha, ok, err := client.CheckRef(src.Owner, src.Repo)(refOverride)
	if err != nil {
		return ghurl.Source{}, err
	}
	if !ok {
		return ghurl.Source{}, fmt.Errorf("%w: %s", ErrRefNotFound, refOverride)
	}
	return ghurl.Build(src.Owner, src.Repo, refOverride, src.Path, sha), nil
}

// defaultDownloadName names the destination of a download after its folder, or
// after the repo for a whole-repo download.
func defaultDownloadName(src ghurl.Source) string {
	if src.Path == "" {
		return src.Repo
	}
	return filepath.Base(src.Path)
}

// planDownload decides how to write the destination (in-place / replace /
// refuse) using the pure policy in internal/dest; only the existence check
// touches the disk.
func planDownload(dest string, force bool) (destpolicy.Plan, error) {
	abs, err := filepath.Abs(dest)
	if err != nil {
		return destpolicy.Plan{}, fmt.Errorf("resolving destination: %w", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return destpolicy.Plan{}, fmt.Errorf("reading the current directory: %w", err)
	}
	_, statErr := os.Lstat(dest)
	return destpolicy.Resolve(abs, cwd, statErr == nil, force), nil
}

// printDownloadHeader prints what a download is about to fetch and where.
func printDownloadHeader(stdout io.Writer, src ghurl.Source, dest string, inPlace bool) {
	folderLabel := src.Path
	if folderLabel == "" {
		folderLabel = "(whole repo)"
	}
	destLabel := dest
	if inPlace {
		destLabel = dest + " (in place — existing files with the same name are overwritten)"
	}
	outln(stdout, "Downloading from GitHub...")
	outf(stdout, "  Repository: %s/%s\n", src.Owner, src.Repo)
	outf(stdout, "  Ref:        %s\n", src.Ref)
	outf(stdout, "  Folder:     %s\n", folderLabel)
	outf(stdout, "  Destination:%s\n\n", " "+destLabel)
}

func metaFrom(src ghurl.Source) meta.Meta {
	return meta.Meta{
		SourceURL:  src.URL,
		Owner:      src.Owner,
		Repo:       src.Repo,
		Branch:     src.Ref,
		FolderPath: src.Path,
		Commit:     src.Commit,
	}
}
