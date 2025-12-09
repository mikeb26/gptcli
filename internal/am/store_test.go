/* Copyright © 2023-2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package am

import (
	"path/filepath"
	"testing"
)

func TestNewMemoryApprovalPolicyStore_Basic(t *testing.T) {
	store := NewMemoryApprovalPolicyStore()
	if store == nil {
		t.Fatal("expected non-nil store")
	}

	if actions, found := store.Check("nonexistent"); len(actions) != 0 || found {
		t.Fatalf("expected (nil,false) for unknown policy, got (%v,%v)", actions, found)
	}
}

func TestMemoryApprovalPolicyStore_SaveAndCheck_File(t *testing.T) {
	store := NewMemoryApprovalPolicyStore()

	id := ApprovalPolicyID(ApprovalSubsysTools, ApprovalGroupFileIO, ApprovalTargetFile, "/tmp/file.txt")

	if actions, found := store.Check(id); len(actions) != 0 || found {
		t.Fatalf("expected (nil,false) before save, got (%v,%v)", actions, found)
	}

	store.Save(id, []ApprovalAction{ApprovalActionRead})
	if actions, found := store.Check(id); !found || len(actions) != 1 || actions[0] != ApprovalActionRead {
		t.Fatalf("expected ([read],true) after saving allow decision, got (%v,%v)", actions, found)
	}
}

func TestMemoryApprovalPolicyStore_DirectoryRecursion(t *testing.T) {
	store := NewMemoryApprovalPolicyStore()

	baseDir := filepath.Join("root", "dir")
	subDir := filepath.Join(baseDir, "subdir")

	baseID := ApprovalPolicyID(ApprovalSubsysTools, ApprovalGroupFileIO, ApprovalTargetDir, baseDir)
	subID := ApprovalPolicyID(ApprovalSubsysTools, ApprovalGroupFileIO, ApprovalTargetDir, subDir)

	store.Save(baseID, []ApprovalAction{ApprovalActionRead})

	if actions, found := store.Check(subID); !found || len(actions) != 1 || actions[0] != ApprovalActionRead {
		t.Fatalf("expected recursive allow for subdirectory, got (%v,%v)", actions, found)
	}
}

func TestMemoryApprovalPolicyStore_DirectoryMostSpecific(t *testing.T) {
	store := NewMemoryApprovalPolicyStore()

	root := filepath.Join("root")
	child := filepath.Join(root, "child")
	grandchild := filepath.Join(child, "grandchild")

	rootID := ApprovalPolicyID(ApprovalSubsysTools, ApprovalGroupFileIO, ApprovalTargetDir, root)
	childID := ApprovalPolicyID(ApprovalSubsysTools, ApprovalGroupFileIO, ApprovalTargetDir, child)
	queryID := ApprovalPolicyID(ApprovalSubsysTools, ApprovalGroupFileIO, ApprovalTargetDir, grandchild)

	store.Save(rootID, []ApprovalAction{ApprovalActionRead})
	store.Save(childID, []ApprovalAction{ApprovalActionWrite})

	if actions, found := store.Check(queryID); !found || len(actions) != 1 || actions[0] != ApprovalActionWrite {
		t.Fatalf("expected most specific (child) policy to apply and grant write, got (%v,%v)", actions, found)
	}
}

func TestMemoryApprovalPolicyStore_DirectoryDifferentSubsysOrGroup(t *testing.T) {
	store := NewMemoryApprovalPolicyStore()

	baseDir := filepath.Join("root", "dir")
	subDir := filepath.Join(baseDir, "subdir")

	baseID := ApprovalPolicyID(ApprovalSubsysTools, ApprovalGroupFileIO, ApprovalTargetDir, baseDir)
	store.Save(baseID, []ApprovalAction{ApprovalActionRead})

	const otherGroup ApprovalGroup = "other-group"
	queryID := ApprovalPolicyID(ApprovalSubsysTools, otherGroup, ApprovalTargetDir, subDir)

	if actions, found := store.Check(queryID); len(actions) != 0 || found {
		t.Fatalf("expected no match for different group, got (%v,%v)", actions, found)
	}
}

func TestMemoryApprovalPolicyStore_DirectoryNotAppliedToFileTarget(t *testing.T) {
	store := NewMemoryApprovalPolicyStore()

	baseDir := filepath.Join("root", "dir")
	subDir := filepath.Join(baseDir, "subdir")

	dirID := ApprovalPolicyID(ApprovalSubsysTools, ApprovalGroupFileIO, ApprovalTargetDir, baseDir)
	store.Save(dirID, []ApprovalAction{ApprovalActionRead})

	fileID := ApprovalPolicyID(ApprovalSubsysTools, ApprovalGroupFileIO, ApprovalTargetFile, subDir)

	if actions, found := store.Check(fileID); len(actions) != 0 || found {
		t.Fatalf("expected directory policy not to apply to file target, got (%v,%v)", actions, found)
	}
}

func TestApprovalPolicyIDAndParsePolicyID_RoundTrip(t *testing.T) {
	subsys := ApprovalSubsysTools
	group := ApprovalGroupFileIO
	target := ApprovalTargetDir
	domain := "path:with:colons"

	id := ApprovalPolicyID(subsys, group, target, domain)

	gotSubsys, gotGroup, gotTarget, gotDomain, ok := parsePolicyID(id)
	if !ok {
		t.Fatalf("expected parsePolicyID to succeed for id %q", id)
	}
	if gotSubsys != subsys || gotGroup != group || gotTarget != target || gotDomain != domain {
		t.Fatalf("round-trip mismatch: got (%v,%v,%v,%q)", gotSubsys, gotGroup, gotTarget, gotDomain)
	}
}

func TestParsePolicyID_Invalid(t *testing.T) {
	if _, _, _, _, ok := parsePolicyID("too:few:parts"); ok {
		t.Fatalf("expected invalid policy ID to return ok=false")
	}
}

func TestParsePolicyID_DomainWithExtraColons(t *testing.T) {
	id := "a:b:c:d:e:f"
	_, _, _, domain, ok := parsePolicyID(id)
	if !ok {
		t.Fatalf("expected ok for id %q", id)
	}
	if domain != "d:e:f" {
		t.Fatalf("expected domain 'd:e:f', got %q", domain)
	}
}

func TestIsPathWithin(t *testing.T) {
	root := filepath.Join("foo", "bar")

	tests := []struct {
		name string
		path string
		root string
		want bool
	}{
		{"same path", filepath.Join("foo", "bar"), root, true},
		{"child path", filepath.Join("foo", "bar", "baz"), root, true},
		{"sibling path", filepath.Join("foo", "bar2"), root, false},
		{"parent path", filepath.Join("foo"), root, false},
		{"empty root", filepath.Join("foo", "bar"), "", false},
		{"empty path", "", root, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPathWithin(tt.path, tt.root)
			if got != tt.want {
				t.Fatalf("isPathWithin(%q,%q) = %v, want %v", tt.path, tt.root, got, tt.want)
			}
		})
	}
}
