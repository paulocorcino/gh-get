// Package dest decides how a download should be written to a destination
// directory. It encodes the in-place / --force / refuse-when-it-contains-cwd
// policy (CONTEXT decision 6a) as a pure function so the rules that govern
// whether existing content is wiped can be tested without touching the network
// or the filesystem.
package dest

import (
	"os"
	"path/filepath"
	"strings"
)

// Action is the decision Resolve reaches for a requested destination.
type Action int

const (
	// WriteInPlace means the destination is the current working directory: write
	// into it, overwriting only colliding paths, never wiping it.
	WriteInPlace Action = iota
	// Replace means the destination should be (re)created from scratch.
	Replace
	// Refuse means the download must not proceed; Reason explains why.
	Refuse
)

// Plan is the outcome of Resolve.
type Plan struct {
	Action Action
	Reason string // populated only when Action == Refuse
}

// Resolve decides how to write absDest given the absolute current working
// directory cwd, whether absDest already exists, and the --force flag. It is
// pure: the caller performs the existence check and passes the result in.
//
// absDest and cwd must both be absolute and cleaned (filepath.Abs output).
func Resolve(absDest, cwd string, exists, force bool) Plan {
	if eqPath(absDest, cwd) {
		return Plan{Action: WriteInPlace}
	}
	if !exists {
		return Plan{Action: Replace}
	}
	if !force {
		return Plan{
			Action: Refuse,
			Reason: "destination already exists: " + absDest + " (use --force to overwrite)",
		}
	}
	if containsDir(absDest, cwd) {
		return Plan{
			Action: Refuse,
			Reason: "refusing to overwrite " + absDest + ": it contains the current directory",
		}
	}
	return Plan{Action: Replace}
}

// containsDir reports whether parent is an ancestor of (or equal to) child —
// i.e. wiping parent would also delete child.
func containsDir(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false // different volumes: cannot contain
	}
	escapes := rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator))
	return !escapes
}

// eqPath compares two cleaned absolute paths, case-insensitively on Windows.
func eqPath(a, b string) bool {
	if os.PathSeparator == '\\' {
		return strings.EqualFold(a, b)
	}
	return a == b
}
