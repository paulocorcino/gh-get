// Command gh-get downloads a single folder from a GitHub repository without
// requiring git, and can update a previously downloaded folder in place.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/paulocorcino/gh-get/internal/manifest"
	"github.com/paulocorcino/gh-get/internal/meta"
)

// version is injected at build time via -ldflags "-X main.version=vX.Y.Z".
var version = "dev"

// Usage errors of the command line; each prints exactly its text.
var (
	ErrInstallArgs         = errors.New("--install only accepts --no-modify-path")
	ErrUpgradeArgs         = errors.New("--upgrade only accepts --force")
	ErrTokenValue          = errors.New("--token requires a value")
	ErrMissingValue        = errors.New("requires a value")
	ErrUnknownOption       = errors.New("unknown option")
	ErrUnexpectedArg       = errors.New("unexpected argument")
	ErrFreezeArgs          = errors.New("freeze only accepts --file")
	ErrFreezeRemote        = errors.New("freeze writes the list file, so --file must be a local path, not a URL")
	ErrRecursiveRef        = errors.New("--recursive cannot be combined with --ref or --branch")
	ErrFileCombined        = errors.New("--file cannot be combined with --recursive, --ref or --branch")
	ErrRecursiveUpdateOnly = errors.New("--recursive is only valid with update")
	ErrFileRef             = errors.New("--file cannot be combined with --ref or --branch (put the ref in each URL)")
	ErrMissingURL          = errors.New("missing GitHub URL")
	ErrRefNotFound         = errors.New("ref not found")
	ErrRefGone             = errors.New("ref no longer exists")
	ErrRecursiveFailed     = errors.New("recursive update operation(s) failed")
	ErrInvalidMarker       = errors.New("invalid " + meta.FileName + ": owner, repo, and branch are required")
)

const usage = `gh-get - download a folder (or a whole repo) from GitHub

Usage:
  gh-get <github-url> [destination] [--force] [--ref REF] [--token TOKEN]
  gh-get -F FILE|URL [--force]
  gh-get update [--ref REF]
  gh-get update -r | --recursive
  gh-get update -F FILE|URL
  gh-get freeze [-F FILE]
  gh-get --install [--no-modify-path]
  gh-get --upgrade [--force]
  gh-get --version | --help

Description:
  Downloads a folder from a GitHub repo (no git required), or the whole repo
  when the URL points at its root. A hidden .gh-get-source file records the
  origin so you can later run "gh-get update" from inside the folder to re-pull.

  A list file (like pip's requirements.txt) downloads many folders at once:
  one GitHub URL per line, optionally followed by a destination (relative to
  the list file); "#" starts a comment. "gh-get -F FILE" downloads the missing
  entries, "gh-get update -F FILE" also updates the existing ones. Running
  "gh-get update" in a directory with a gh-get.txt (and no .gh-get-source)
  uses that file automatically. "gh-get freeze" scans the current directory
  for downloaded folders and creates or updates gh-get.txt to list them.

  The list file may also live in a GitHub repository: pass its URL to -F and it
  is read into memory, never saved. Destinations then resolve against the
  current directory.

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

  # Download / update everything listed in gh-get.txt
  gh-get -F gh-get.txt
  gh-get update

  # Record every folder already downloaded under the current dir in gh-get.txt
  gh-get freeze

Options:
      --install      Copy gh-get into a per-user bin dir and put it on your
                     PATH (no admin required): the user PATH on Windows, your
                     shell profile on Linux/macOS (--no-modify-path: only print
                     the line to add)
      --upgrade      Replace this gh-get with the latest GitHub release
                     (checksum-verified; --force also reinstalls the same
                     version or replaces a dev build)
  -f, --force        Overwrite the destination if it already exists
  -F, --file FILE    Download (or, with "update", update) every entry of a
                     list file such as gh-get.txt, or of a list file in a
                     GitHub repo given by its URL
      --ref REF      Branch, tag or commit to use; overrides the ref in the URL.
                     With "update", switches the folder to this ref.
  -r, --recursive    With "update", update every gh-get folder at or below the
                     current directory (cannot be combined with --ref)
      --token TOKEN  GitHub token (else $GITHUB_TOKEN / $GH_TOKEN; optional)
  -h, --help         Show this help
  -v, --version      Show version
`

func main() {
	removeReplacedBinary()
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "Error: "+err.Error())
		os.Exit(1) //nolint:forbidigo // process exit code; main returns nothing
	}
}

// run executes the command line args, printing to stdout and stderr; the
// caller turns a returned error into the exit code.
func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		outf(stdout, "%s", usage)
		return nil
	}

	switch args[0] {
	case "-h", "--help":
		outf(stdout, "%s", usage)
		return nil
	case "-v", "--version":
		outln(stdout, "gh-get "+version)
		return nil
	case "--install":
		modifyPath, err := parseInstallArgs(args[1:])
		if err != nil {
			return err
		}
		return runInstall(modifyPath, stdout)
	case "--upgrade", "--self-update": // --self-update is the old name
		force, err := parseUpgradeArgs(args[1:])
		if err != nil {
			return err
		}
		return runSelfUpdate(force, stdout)
	}

	cl, err := parseCommandLine(args)
	if err != nil {
		return err
	}
	return dispatch(cl, stdout, stderr)
}

// dispatch runs the command a parsed command line names: freeze, update, a
// download over a list file, or a single download.
func dispatch(cl commandLine, stdout, stderr io.Writer) error {
	switch cl.command() {
	case "freeze":
		listFile, err := cl.freezeListFile()
		if err != nil {
			return err
		}
		return runFreeze(listFile, stdout)
	case "update":
		return runUpdateCommand(cl, stdout, stderr)
	}
	if cl.Recursive {
		return ErrRecursiveUpdateOnly
	}
	if cl.ListFile != "" {
		req, err := cl.listRequest()
		if err != nil {
			return err
		}
		return runList(req, stdout, stderr)
	}
	if len(cl.Positional) == 0 {
		outf(stdout, "%s", usage)
		return ErrMissingURL
	}
	req, err := cl.downloadRequest()
	if err != nil {
		return err
	}
	return runDownload(req, stdout, stderr)
}

// runUpdateCommand runs "update": over a list file, recursively, or on the
// current folder.
func runUpdateCommand(cl commandLine, stdout, stderr io.Writer) error {
	if err := cl.checkUpdate(); err != nil {
		return err
	}
	token := resolveToken(cl.Token)
	switch {
	case cl.ListFile != "":
		return runList(listRequest{Path: cl.ListFile, Token: token, Refresh: true, Force: cl.Force}, stdout, stderr)
	case cl.Recursive:
		return runRecursiveUpdate(token, stdout, stderr)
	default:
		return runUpdate(token, cl.Ref, stdout, stderr)
	}
}

// parseInstallArgs reads the arguments after --install and reports whether
// --install may modify the PATH.
func parseInstallArgs(args []string) (bool, error) {
	modifyPath := true
	for _, a := range args {
		if a != "--no-modify-path" {
			return false, ErrInstallArgs
		}
		modifyPath = false
	}
	return modifyPath, nil
}

// parseUpgradeArgs reads the arguments after --upgrade and reports
// whether --force was given.
func parseUpgradeArgs(args []string) (bool, error) {
	force := false
	for _, a := range args {
		if a != "-f" && a != "--force" {
			return false, ErrUpgradeArgs
		}
		force = true
	}
	return force, nil
}

// optToken is the option that passes the GitHub token.
const optToken = "--token"

// commandLine is the parsed command line of every command but --install and
// --upgrade: its flags and its positional arguments.
type commandLine struct {
	Force      bool
	Recursive  bool
	Token      string
	Ref        string
	ListFile   string
	Positional []string
}

// parseCommandLine reads the flags and positional arguments of args.
func parseCommandLine(args []string) (commandLine, error) {
	var cl commandLine
	for i := 0; i < len(args); i++ {
		a := args[i]
		if target := cl.valueOf(a); target != nil {
			v, ok := optionValue(args, i)
			if !ok {
				return cl, missingValue(a)
			}
			*target = v
			i++
			continue
		}
		if err := cl.apply(a); err != nil {
			return cl, err
		}
	}
	return cl, nil
}

// valueOf returns the field set by an option that takes its value from the
// next argument, or nil when name is not such an option.
func (cl *commandLine) valueOf(name string) *string {
	switch name {
	case optToken:
		return &cl.Token
	case "--ref", "--branch":
		return &cl.Ref
	case "-F", "--file":
		return &cl.ListFile
	}
	return nil
}

// apply records one argument that is not an option followed by its value.
func (cl *commandLine) apply(a string) error {
	if cl.applyInline(a) {
		return nil
	}
	switch {
	case a == "-f" || a == "--force":
		cl.Force = true
	case a == "-r" || a == "--recursive":
		cl.Recursive = true
	case strings.HasPrefix(a, "-"):
		return fmt.Errorf("%w: %s", ErrUnknownOption, a)
	default:
		cl.Positional = append(cl.Positional, a)
	}
	return nil
}

// applyInline records an option written as --name=value and reports whether
// a was one.
func (cl *commandLine) applyInline(a string) bool {
	name, v, ok := strings.Cut(a, "=")
	if !ok {
		return false
	}
	switch name {
	case optToken:
		cl.Token = v
	case "--ref", "--branch":
		cl.Ref = v
	case "--file":
		cl.ListFile = v
	default:
		return false
	}
	return true
}

// optionValue returns the argument after args[i], the value of an option that
// takes one, and false when args ends at i.
func optionValue(args []string, i int) (string, bool) {
	if i+1 >= len(args) {
		return "", false
	}
	return args[i+1], true
}

// missingValue is the error of option name given without its value.
func missingValue(name string) error {
	if name == optToken {
		return ErrTokenValue
	}
	return fmt.Errorf("%s %w", name, ErrMissingValue)
}

// command is the subcommand the first positional argument names, or "".
func (cl *commandLine) command() string {
	if len(cl.Positional) == 0 {
		return ""
	}
	return cl.Positional[0]
}

// freezeListFile checks the arguments of "freeze" and returns the list file
// it writes.
func (cl *commandLine) freezeListFile() (string, error) {
	if len(cl.Positional) > 1 {
		return "", fmt.Errorf("%w: %s", ErrUnexpectedArg, cl.Positional[1])
	}
	if cl.Force || cl.Recursive || cl.Ref != "" {
		return "", ErrFreezeArgs
	}
	if cl.ListFile == "" {
		return manifest.DefaultFile, nil
	}
	if isRemoteList(cl.ListFile) {
		return "", ErrFreezeRemote
	}
	return cl.ListFile, nil
}

// checkUpdate checks the arguments of "update".
func (cl *commandLine) checkUpdate() error {
	if len(cl.Positional) > 1 {
		return fmt.Errorf("%w: %s", ErrUnexpectedArg, cl.Positional[1])
	}
	if cl.Recursive && cl.Ref != "" {
		return ErrRecursiveRef
	}
	if cl.ListFile != "" && (cl.Recursive || cl.Ref != "") {
		return ErrFileCombined
	}
	return nil
}

// listRequest checks the arguments of a download over a list file (-F
// without "update") and returns the request.
func (cl *commandLine) listRequest() (listRequest, error) {
	if len(cl.Positional) > 0 {
		return listRequest{}, fmt.Errorf("%w with --file: %s", ErrUnexpectedArg, cl.Positional[0])
	}
	if cl.Ref != "" {
		return listRequest{}, ErrFileRef
	}
	return listRequest{Path: cl.ListFile, Token: resolveToken(cl.Token), Force: cl.Force}, nil
}

// downloadRequest checks the arguments of a single download (a URL and an
// optional destination) and returns the request.
func (cl *commandLine) downloadRequest() (downloadRequest, error) {
	if len(cl.Positional) > 2 {
		return downloadRequest{}, fmt.Errorf("%w: %s", ErrUnexpectedArg, cl.Positional[2])
	}
	req := downloadRequest{URL: cl.Positional[0], Force: cl.Force, Token: resolveToken(cl.Token), Ref: cl.Ref}
	if len(cl.Positional) == 2 {
		req.Dest = cl.Positional[1]
	}
	return req, nil
}

func resolveToken(flag string) string {
	if flag != "" {
		return flag
	}
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t
	}
	if t := os.Getenv("GH_TOKEN"); t != "" {
		return t
	}
	return tokenFromGHCLI()
}

// tokenFromGHCLI returns the token stored by an authenticated GitHub CLI
// (`gh auth token`), or "" if gh is absent, not logged in, or slow to respond.
// Any failure is silent so callers fall back to anonymous access.
func tokenFromGHCLI() string {
	if _, err := exec.LookPath("gh"); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "gh", "auth", "token").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// warnTo returns a notice sink that prints each non-fatal notice to w.
func warnTo(w io.Writer) func(string) {
	return func(msg string) {
		outln(w, "warning: "+msg)
	}
}

// warn prints a non-fatal notice to the process's stderr. It is for leaf
// cleanup helpers (closing or removing a file) that sit below any writer.
func warn(msg string) {
	warnTo(os.Stderr)(msg)
}

// outf writes formatted terminal output to w, as fmt.Fprintf.
func outf(w io.Writer, format string, a ...any) {
	fmt.Fprintf(w, format, a...) //nolint:errcheck // terminal output, like printing to os.Stdout: a failed write to stdout/stderr has no one left to report to
}

// outln writes one line of terminal output to w, as fmt.Fprintln.
func outln(w io.Writer, a ...any) {
	fmt.Fprintln(w, a...) //nolint:errcheck // terminal output, like printing to os.Stdout: a failed write to stdout/stderr has no one left to report to
}

// displayPath is path relative to root for output, "." for root itself.
func displayPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return "."
	}
	return rel
}
