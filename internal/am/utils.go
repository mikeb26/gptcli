/* Copyright © 2023-2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package am

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ApprovalPolicyID constructs a stable identifier for an approval policy
// decision. The components are intentionally generic so that different
// subsystems and groupings (e.g. all file tools) can share policy
// semantics.
//
// A common pattern would be to use, for example:
//
//	subsys = "tool"
//	group  = "fileio"
//	target = "file" or "directory"
//	domain = concrete path (filename or directory name)
func ApprovalPolicyID(subsys ApprovalSubsys, group ApprovalGroup,
	target ApprovalTarget, domain string) string {

	return fmt.Sprintf("%v:%v:%v:%v", subsys, group, target, domain)
}

// parsePolicyID extracts the four components of a policy identifier.
// It returns ok=false if the identifier does not match the expected
// format.
func parsePolicyID(id string) (subsys ApprovalSubsys, group ApprovalGroup,
	target ApprovalTarget, domain string, ok bool) {
	parts := strings.SplitN(id, ":", 4)
	if len(parts) != 4 {
		return "", "", "", "", false
	}
	return ApprovalSubsys(parts[0]), ApprovalGroup(parts[1]),
		ApprovalTarget(parts[2]), parts[3], true
}

// isPathWithin reports whether "path" is within "root" when interpreted
// as filesystem paths. This is used to support recursive directory
// policies in a platform-agnostic way.
func isPathWithin(path, root string) bool {
	if root == "" {
		return false
	}

	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(root)

	if cleanPath == cleanRoot {
		return true
	}

	// Ensure root ends with a path separator so we don't get spurious
	// matches (e.g. "/foo/bar2" shouldn't match root "/foo/bar").
	if !strings.HasSuffix(cleanRoot, string(filepath.Separator)) {
		cleanRoot += string(filepath.Separator)
	}
	return strings.HasPrefix(cleanPath, cleanRoot)
}
