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
// sure that directory is on the user's PATH: the user PATH on Windows, the
// shell profile elsewhere (unless modifyPath is false, which only prints the
// line to add). It never requires elevated privileges.
func runInstall(modifyPath bool) error {
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

	return ensureOnPath(dir, modifyPath)
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
// edits the user PATH; elsewhere it appends a guarded line to the profile of
// the user's shell. With modifyPath false it only prints what to add.
func ensureOnPath(dir string, modifyPath bool) error {
	if pathContains(os.Getenv("PATH"), dir) {
		fmt.Println("\n'" + dir + "' is already on your PATH. Run: gh-get --version")
		return nil
	}

	if runtime.GOOS == "windows" {
		if !modifyPath {
			fmt.Println("\n'" + dir + "' is not on your PATH yet; add it to your user PATH.")
			return nil
		}
		added, err := addToWindowsUserPath(dir)
		if err != nil {
			return err
		}
		if added {
			fmt.Println("\nAdded '" + dir + "' to your user PATH.")
		} else {
			fmt.Println("\n'" + dir + "' is already on your user PATH.")
		}
		fmt.Println("Open a NEW terminal, then run: gh-get --version")
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	prof := shellProfile(os.Getenv("SHELL"), home, os.Getenv("ZDOTDIR"), runtime.GOOS, dir)
	if !modifyPath {
		fmt.Println("\n'" + dir + "' is not on your PATH yet.")
		fmt.Println("Add this to " + prof.File + ":")
		fmt.Println("\n  " + prof.Line)
		fmt.Println("\nThen restart your shell.")
		return nil
	}
	added, err := addToProfile(prof)
	if err != nil {
		return fmt.Errorf("updating %s: %w", prof.File, err)
	}
	if added {
		fmt.Println("\nAdded '" + dir + "' to your PATH in " + prof.File)
	} else {
		fmt.Println("\n" + prof.File + " already adds '" + dir + "' to your PATH.")
	}
	fmt.Println("Open a NEW terminal, then run: gh-get --version")
	return nil
}

// profileMarker tags the line gh-get adds, so re-running --install is a no-op.
const profileMarker = "# added by gh-get --install"

// profileEdit is the startup file of a shell and the line that puts a
// directory on PATH in that shell's syntax.
type profileEdit struct {
	File string
	Line string
}

// shellProfile picks the startup file of the user's login shell (from $SHELL)
// and the PATH line for it. The POSIX line guards against duplicate entries
// when the file is sourced more than once.
func shellProfile(shell, home, zdotdir, goos, dir string) profileEdit {
	q := shQuote(dir)
	posix := `case ":$PATH:" in *:` + q + `:*) ;; *) export PATH=` + q + `:"$PATH" ;; esac`
	switch filepath.Base(shell) {
	case "zsh":
		if zdotdir == "" {
			zdotdir = home
		}
		return profileEdit{filepath.Join(zdotdir, ".zshrc"), posix}
	case "bash":
		// macOS terminals start login shells, which read .bash_profile only.
		if goos == "darwin" {
			return profileEdit{filepath.Join(home, ".bash_profile"), posix}
		}
		return profileEdit{filepath.Join(home, ".bashrc"), posix}
	case "fish":
		return profileEdit{
			filepath.Join(home, ".config", "fish", "conf.d", "gh-get.fish"),
			"contains -- " + q + " $PATH; or set -gx PATH " + q + " $PATH",
		}
	default:
		return profileEdit{filepath.Join(home, ".profile"), posix}
	}
}

// addToProfile appends the marked PATH line to the profile, creating it if
// needed. It reports false when an earlier --install already added it.
func addToProfile(p profileEdit) (bool, error) {
	existing, err := os.ReadFile(p.File)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if strings.Contains(string(existing), profileMarker) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(p.File), 0o755); err != nil {
		return false, err
	}
	f, err := os.OpenFile(p.File, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	block := "\n" + profileMarker + "\n" + p.Line + "\n"
	if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
		block = "\n" + block
	}
	if _, err := f.WriteString(block); err != nil {
		f.Close()
		return false, err
	}
	return true, f.Close()
}

// shQuote single-quotes s for POSIX shells and fish.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
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
