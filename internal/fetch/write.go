package fetch

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

var errUnsafePath = errors.New("unsafe path escapes destination")

// safeJoin joins root and rel, guaranteeing the result stays within root. It
// rejects entries that would escape via "..", protecting tarball extraction.
func safeJoin(root, rel string) (string, error) {
	root = filepath.Clean(root)
	dest := filepath.Join(root, filepath.FromSlash(rel))
	r, err := filepath.Rel(root, dest)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: %s", errUnsafePath, rel)
	}
	return dest, nil
}

// writeFile writes a regular file at dest with the given exec bit, creating
// parent directories. The exec bit is only meaningful on non-Windows systems.
func writeFile(dest string, data []byte, exec bool) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("creating directory for %s: %w", dest, err)
	}
	mode := os.FileMode(0o644)
	if exec && runtime.GOOS != "windows" {
		mode = 0o755
	}
	if err := os.WriteFile(dest, data, mode); err != nil {
		return fmt.Errorf("writing %s: %w", dest, err)
	}
	return nil
}

// writeSymlink tries to create a symlink at dest pointing to target. When the
// platform refuses (e.g. Windows without privilege), it degrades to a regular
// file containing the target path and reports the degradation via warn.
func writeSymlink(dest, target string, warn func(string)) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("creating directory for %s: %w", dest, err)
	}
	os.Remove(dest) //nolint:errcheck,gosec // a missing or undeletable stale entry surfaces at Symlink or WriteFile below
	if err := os.Symlink(target, dest); err != nil {
		if warn != nil {
			warn(fmt.Sprintf("symlink not supported, wrote plain file: %s -> %s", dest, target))
		}
		if err := os.WriteFile(dest, []byte(target), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", dest, err)
		}
	}
	return nil
}

// publishTree copies the tree rooted at src into dst, routing every entry
// through writeFile/writeSymlink so exec-bit and symlink-degradation handling
// lives in exactly one place. Existing files at colliding paths are overwritten;
// files in dst that have no counterpart in src are left untouched (merge).
func publishTree(src, dst string, warn func(string)) error {
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("relating %s to %s: %w", path, src, err)
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)

		switch {
		case info.IsDir():
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("creating %s: %w", target, err)
			}
			return nil
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return fmt.Errorf("reading link %s: %w", path, err)
			}
			return writeSymlink(target, link, warn)
		default:
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("reading %s: %w", path, err)
			}
			return writeFile(target, data, info.Mode()&0o111 != 0)
		}
	})
	if err != nil {
		return fmt.Errorf("publishing %s: %w", src, err)
	}
	return nil
}

// clearContents removes every entry inside dir without removing dir itself. The
// directory is kept so dir may safely be the current working directory (which on
// Windows cannot be deleted while in use).
func clearContents(dir string) error {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading %s: %w", dir, err)
	}
	for _, e := range ents {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return fmt.Errorf("removing %s: %w", e.Name(), err)
		}
	}
	return nil
}

// listFiles returns the slash-separated paths, relative to root, of every
// non-directory entry under root, sorted.
func listFiles(root string) ([]string, error) {
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("relating %s to %s: %w", path, root, err)
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(out)
	if err != nil {
		return out, fmt.Errorf("listing %s: %w", root, err)
	}
	return out, nil
}

// pruneStale deletes from dir each file in prev that keep does not list, then
// removes directories left empty by those deletions (never dir itself). Only
// tracked files are touched; paths escaping dir are ignored.
func pruneStale(dir string, prev, keep []string) error {
	kept := make(map[string]bool, len(keep))
	for _, k := range keep {
		kept[k] = true
	}
	root := filepath.Clean(dir)
	for _, p := range prev {
		if kept[p] {
			continue
		}
		target, err := safeJoin(root, p)
		if err != nil || target == root {
			continue
		}
		if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("removing stale %s: %w", target, err)
		}
		for d := filepath.Dir(target); d != root && strings.HasPrefix(d, root); d = filepath.Dir(d) {
			if os.Remove(d) != nil {
				break // not empty (or already gone): stop climbing
			}
		}
	}
	return nil
}
