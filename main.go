// Command gh-get downloads a single folder from a GitHub repository without
// requiring git, and can update a previously downloaded folder in place.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/paulocorcino/gh-get/internal/fetch"
	"github.com/paulocorcino/gh-get/internal/ghurl"
	"github.com/paulocorcino/gh-get/internal/meta"
)

// version is injected at build time via -ldflags "-X main.version=vX.Y.Z".
var version = "dev"

const usage = `gh-get - download a folder (or a whole repo) from GitHub

Usage:
  gh-get <github-url> [destination] [--force] [--ref REF] [--token TOKEN]
  gh-get update [--ref REF]
  gh-get --install
  gh-get --version | --help

Description:
  Downloads a folder from a GitHub repo (no git required), or the whole repo
  when the URL points at its root. A hidden .gh-get-source file records the
  origin so you can later run "gh-get update" from inside the folder to re-pull.

URL formats:
  https://github.com/OWNER/REPO                       (whole repo, default branch)
  https://github.com/OWNER/REPO/tree/BRANCH           (whole repo at BRANCH)
  https://github.com/OWNER/REPO/tree/BRANCH/PATH/TO/FOLDER
  https://github.com/OWNER/REPO/blob/BRANCH/PATH/TO/FOLDER

Examples:
  # Download one folder
  gh-get https://github.com/octo-org/sample-repo/tree/main/docs/guide

  # Same folder into ./guide, overwriting if it already exists
  gh-get https://github.com/octo-org/sample-repo/tree/main/docs/guide ./guide --force

  # Pick a different branch/tag than the URL
  gh-get https://github.com/octo-org/sample-repo/tree/main/docs/guide --ref v2.0.0

  # Download the whole repo (default branch)
  gh-get https://github.com/octo-org/sample-repo

  # Re-pull the latest content from inside a downloaded folder
  cd ./guide && gh-get update

Options:
      --install      Copy gh-get into a per-user bin dir on your PATH
                     (no admin required) so it can be run from anywhere
  -f, --force        Overwrite the destination if it already exists
      --ref REF      Branch, tag or commit to use; overrides the ref in the URL.
                     With "update", switches the folder to this ref.
      --token TOKEN  GitHub token (else $GITHUB_TOKEN / $GH_TOKEN; optional)
  -h, --help         Show this help
  -v, --version      Show version
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Error: "+err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Print(usage)
		return nil
	}

	switch args[0] {
	case "-h", "--help":
		fmt.Print(usage)
		return nil
	case "-v", "--version":
		fmt.Println("gh-get " + version)
		return nil
	case "--install":
		if len(args) > 1 {
			return fmt.Errorf("--install takes no other arguments")
		}
		return runInstall()
	}

	var (
		force      bool
		tokenFlag  string
		refFlag    string
		positional []string
	)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-f" || a == "--force":
			force = true
		case a == "--token":
			if i+1 >= len(args) {
				return fmt.Errorf("--token requires a value")
			}
			i++
			tokenFlag = args[i]
		case strings.HasPrefix(a, "--token="):
			tokenFlag = strings.TrimPrefix(a, "--token=")
		case a == "--ref" || a == "--branch":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", a)
			}
			i++
			refFlag = args[i]
		case strings.HasPrefix(a, "--ref="):
			refFlag = strings.TrimPrefix(a, "--ref=")
		case strings.HasPrefix(a, "--branch="):
			refFlag = strings.TrimPrefix(a, "--branch=")
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("unknown option: %s", a)
		default:
			positional = append(positional, a)
		}
	}

	token := resolveToken(tokenFlag)

	if len(positional) > 0 && positional[0] == "update" {
		if len(positional) > 1 {
			return fmt.Errorf("unexpected argument: %s", positional[1])
		}
		return runUpdate(token, refFlag)
	}

	if len(positional) == 0 {
		fmt.Print(usage)
		return fmt.Errorf("missing GitHub URL")
	}
	if len(positional) > 2 {
		return fmt.Errorf("unexpected argument: %s", positional[2])
	}

	url := positional[0]
	dest := ""
	if len(positional) == 2 {
		dest = positional[1]
	}
	return runDownload(url, dest, force, token, refFlag)
}

func runDownload(url, dest string, force bool, token, refOverride string) error {
	client := fetch.New(token)

	partial, err := ghurl.Parse(url)
	if err != nil {
		return err
	}
	src, err := partial.Resolve(
		client.CheckRef(partial.Owner, partial.Repo),
		func() (string, error) { return client.DefaultBranch(partial.Owner, partial.Repo) },
	)
	if err != nil {
		return err
	}

	// --ref takes precedence over the branch embedded in the URL. The URL's ref
	// is still resolved above so the folder path can be split correctly.
	if refOverride != "" {
		sha, ok, err := client.CheckRef(src.Owner, src.Repo)(refOverride)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("ref not found: %s", refOverride)
		}
		src = ghurl.Build(src.Owner, src.Repo, refOverride, src.Path, sha)
	}

	if dest == "" {
		name := src.Repo // whole-repo download
		if src.Path != "" {
			name = filepath.Base(src.Path)
		}
		dest = "./" + name
	}

	if _, err := os.Lstat(dest); err == nil {
		if !force {
			return fmt.Errorf("destination already exists: %s (use --force to overwrite)", dest)
		}
		if err := os.RemoveAll(dest); err != nil {
			return err
		}
	}

	folderLabel := src.Path
	if folderLabel == "" {
		folderLabel = "(whole repo)"
	}
	fmt.Println("Downloading from GitHub...")
	fmt.Printf("  Repository: %s/%s\n", src.Owner, src.Repo)
	fmt.Printf("  Ref:        %s\n", src.Ref)
	fmt.Printf("  Folder:     %s\n", folderLabel)
	fmt.Printf("  Destination:%s\n\n", " "+dest)

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	if err := client.Download(src, dest, warn); err != nil {
		os.RemoveAll(dest) // don't leave a half-written/empty folder behind
		return err
	}
	if err := meta.Write(dest, metaFrom(src)); err != nil {
		return err
	}

	fmt.Println("Done:")
	fmt.Println("  " + dest)
	return nil
}

func runUpdate(token, refOverride string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	m, err := meta.Read(cwd)
	if err != nil {
		if os.IsNotExist(err) {
			// Not a gh-get folder; nothing to do (parity with original).
			return nil
		}
		return err
	}
	if m.SourceURL == "" {
		return nil
	}

	client := fetch.New(token)

	// --ref switches the folder to a different branch/tag; otherwise stay on the
	// ref recorded in the marker.
	ref := m.Branch
	if refOverride != "" {
		ref = refOverride
	}

	sha, ok, err := client.CheckRef(m.Owner, m.Repo)(ref)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("ref no longer exists: %s", ref)
	}
	// Short-circuit only when staying on the same ref at the same commit.
	if ref == m.Branch && sha == m.Commit && m.Commit != "" {
		fmt.Println("Already up to date.")
		return nil
	}

	src := ghurl.Build(m.Owner, m.Repo, ref, m.FolderPath, sha)

	fmt.Println("Updating current folder via gh-get...")
	fmt.Printf("  Source: %s\n", m.SourceURL)
	fmt.Printf("  Dest:   %s\n\n", cwd)

	// Download to a temp dir first so a failure never destroys local content.
	tmp, err := os.MkdirTemp("", "gh-get-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	if err := client.Download(src, tmp, warn); err != nil {
		return err
	}

	if err := clearExcept(cwd, meta.FileName); err != nil {
		return err
	}
	if err := copyTree(tmp, cwd); err != nil {
		return err
	}
	if err := meta.Write(cwd, metaFrom(src)); err != nil {
		return err
	}

	fmt.Println("Updated:")
	fmt.Println("  " + cwd)
	return nil
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

func resolveToken(flag string) string {
	if flag != "" {
		return flag
	}
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t
	}
	return os.Getenv("GH_TOKEN")
}

func warn(msg string) {
	fmt.Fprintln(os.Stderr, "warning: "+msg)
}

// clearExcept removes every entry in dir except the one named keep.
func clearExcept(dir, keep string) error {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range ents {
		if e.Name() == keep {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// copyTree copies the contents of src into dst, preserving file modes and
// symlinks where the platform allows.
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)

		switch {
		case info.IsDir():
			return os.MkdirAll(target, 0o755)
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			_ = os.Remove(target)
			if err := os.Symlink(link, target); err != nil {
				return os.WriteFile(target, []byte(link), 0o644)
			}
			return nil
		default:
			return copyFile(path, target, info.Mode())
		}
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
