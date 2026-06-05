package fetch

import "strings"

// folderMember reports whether entryPath (a repo-relative path) lies inside
// folder and, if so, returns its path relative to folder. An empty folder means
// the whole repo, so every entry is a member. An entry whose path is exactly
// folder is not a member (rel == ""), since there is no file to write at the
// destination root; this keeps the Trees and tarball paths in agreement.
//
// folder is expected pre-trimmed of leading/trailing slashes.
func folderMember(folder, entryPath string) (rel string, ok bool) {
	if folder == "" {
		return entryPath, true
	}
	if strings.HasPrefix(entryPath, folder+"/") {
		return strings.TrimPrefix(entryPath, folder+"/"), true
	}
	return "", false
}
