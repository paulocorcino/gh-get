package fetch

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// safeJoin joins root and rel, guaranteeing the result stays within root. It
// rejects entries that would escape via "..", protecting tarball extraction.
func safeJoin(root, rel string) (string, error) {
	root = filepath.Clean(root)
	dest := filepath.Join(root, filepath.FromSlash(rel))
	r, err := filepath.Rel(root, dest)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("unsafe path escapes destination: %s", rel)
	}
	return dest, nil
}

// writeFile writes a regular file at dest with the given exec bit, creating
// parent directories. The exec bit is only meaningful on non-Windows systems.
func writeFile(dest string, data []byte, exec bool) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if exec && runtime.GOOS != "windows" {
		mode = 0o755
	}
	return os.WriteFile(dest, data, mode)
}

// writeSymlink tries to create a symlink at dest pointing to target. When the
// platform refuses (e.g. Windows without privilege), it degrades to a regular
// file containing the target path and reports the degradation via warn.
func writeSymlink(dest, target string, warn func(string)) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	_ = os.Remove(dest)
	if err := os.Symlink(target, dest); err != nil {
		if warn != nil {
			warn(fmt.Sprintf("symlink not supported, wrote plain file: %s -> %s", dest, target))
		}
		return os.WriteFile(dest, []byte(target), 0o644)
	}
	return nil
}
