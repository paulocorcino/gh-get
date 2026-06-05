package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// copyFile copies src to dst with the given mode, creating parent directories.
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

// installDir returns the per-user directory gh-get installs itself into. None of
// these locations require administrator/root rights.
func installDir() (string, error) {
	if runtime.GOOS == "windows" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			return "", fmt.Errorf("LOCALAPPDATA is not set")
		}
		return filepath.Join(base, "Programs", "gh-get"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "bin"), nil
}

func installBinName() string {
	if runtime.GOOS == "windows" {
		return "gh-get.exe"
	}
	return "gh-get"
}

// runInstall copies the running binary into a per-user bin directory and makes
// sure that directory is on the user's PATH (editing it on Windows, advising on
// Unix). It never requires elevated privileges.
func runInstall() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	dir, err := installDir()
	if err != nil {
		return err
	}
	dest := filepath.Join(dir, installBinName())

	if sameFile(exe, dest) {
		fmt.Println("Already installed at:")
		fmt.Println("  " + dest)
	} else {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := copyFile(exe, dest, 0o755); err != nil {
			return fmt.Errorf("copying binary to %s: %w", dest, err)
		}
		fmt.Println("Installed gh-get to:")
		fmt.Println("  " + dest)
	}

	return ensureOnPath(dir)
}

// sameFile reports whether a and b resolve to the same existing file.
func sameFile(a, b string) bool {
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(ai, bi)
}

// ensureOnPath makes sure dir is reachable from the user's PATH. On Windows it
// edits the user PATH directly; elsewhere it only prints instructions, leaving
// shell rc files untouched.
func ensureOnPath(dir string) error {
	if pathContains(os.Getenv("PATH"), dir) {
		fmt.Println("\n'" + dir + "' is already on your PATH. Run: gh-get --version")
		return nil
	}

	if runtime.GOOS == "windows" {
		added, err := addToWindowsUserPath(dir)
		if err != nil {
			return err
		}
		if added {
			fmt.Println("\nAdded '" + dir + "' to your user PATH.")
			fmt.Println("Open a NEW terminal, then run: gh-get --version")
		} else {
			fmt.Println("\n'" + dir + "' is already on your user PATH.")
			fmt.Println("Open a NEW terminal, then run: gh-get --version")
		}
		return nil
	}

	fmt.Println("\n'" + dir + "' is not on your PATH yet.")
	fmt.Println("Add this line to your shell profile (~/.bashrc, ~/.zshrc, ~/.profile):")
	fmt.Println("\n  export PATH=\"" + dir + ":$PATH\"")
	fmt.Println("\nThen restart your shell, or run it now in the current one.")
	return nil
}

// pathContains reports whether dir is one of the entries in a PATH-style string.
func pathContains(pathEnv, dir string) bool {
	if pathEnv == "" {
		return false
	}
	dir = filepath.Clean(dir)
	for _, p := range filepath.SplitList(pathEnv) {
		if p == "" {
			continue
		}
		if eqPath(filepath.Clean(p), dir) {
			return true
		}
	}
	return false
}

// eqPath compares two paths, case-insensitively on Windows.
func eqPath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// addToWindowsUserPath appends dir to the user (HKCU) PATH via PowerShell, which
// applies immediately to new processes and needs no admin rights. It reports
// whether the directory was newly added. Using SetEnvironmentVariable avoids the
// 1024-character truncation bug of setx.
func addToWindowsUserPath(dir string) (bool, error) {
	q := "'" + strings.ReplaceAll(dir, "'", "''") + "'"
	script := `$d = ` + q + `
$p = [Environment]::GetEnvironmentVariable('Path', 'User')
if (-not $p) { $p = '' }
$parts = $p -split ';' | Where-Object { $_ -ne '' }
if ($parts -icontains $d) { 'present'; exit 0 }
$new = if ($p.TrimEnd(';') -eq '') { $d } else { $p.TrimEnd(';') + ';' + $d }
[Environment]::SetEnvironmentVariable('Path', $new, 'User')
'added'`

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("updating user PATH: %w", err)
	}
	return strings.Contains(string(out), "added"), nil
}
