/* Copyright © 2023-2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package am

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewJSONApprovalPolicyStore_EmptyFileCreation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policies.json")

	store, err := NewJSONApprovalPolicyStore(path)
	if err != nil {
		t.Fatalf("NewJSONApprovalPolicyStore(%q) error = %v, want nil", path, err)
	}
	if store == nil {
		t.Fatalf("expected non-nil store")
	}

	if actions, found := store.Check("nonexistent"); len(actions) != 0 || found {
		t.Fatalf("expected (nil,false) for unknown policy, got (%v,%v)", actions, found)
	}

	if _, err := os.Stat(path); err == nil || !os.IsNotExist(err) {
		// Constructor should not create the file until something is saved.
		t.Fatalf("expected JSON file %q to not exist until first Save", path)
	}
}

func TestNewJSONApprovalPolicyStore_LoadExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policies.json")

	initial := map[string][]ApprovalAction{
		"tools:fileio:file:/tmp/foo.txt": {ApprovalActionRead},
	}
	data, err := json.Marshal(initial)
	if err != nil {
		t.Fatalf("failed to marshal initial data: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("failed to seed JSON file: %v", err)
	}

	store, err := NewJSONApprovalPolicyStore(path)
	if err != nil {
		t.Fatalf("NewJSONApprovalPolicyStore(%q) error = %v, want nil", path, err)
	}

	if actions, found := store.Check("tools:fileio:file:/tmp/foo.txt"); !found || len(actions) != 1 || actions[0] != ApprovalActionRead {
		t.Fatalf("expected stored policy to be loaded, got (%v,%v)", actions, found)
	}
}

func TestNewJSONApprovalPolicyStore_InvalidPath(t *testing.T) {
	dir := t.TempDir()
	// Use the temp dir itself as the path; this should be rejected
	// because it is a directory, not a file.
	if _, err := NewJSONApprovalPolicyStore(dir); err == nil {
		t.Fatalf("expected error when filename points to a directory")
	}
}

func TestJSONApprovalPolicyStore_SaveAndPersist_File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policies.json")

	store, err := NewJSONApprovalPolicyStore(path)
	if err != nil {
		t.Fatalf("NewJSONApprovalPolicyStore(%q) error = %v, want nil", path, err)
	}

	id := ApprovalPolicyID(ApprovalSubsysTools, ApprovalGroupFileIO, ApprovalTargetFile, "/tmp/file.txt")

	store.Save(id, []ApprovalAction{ApprovalActionRead})

	// Verify in-memory behavior matches MemoryApprovalPolicyStore
	if actions, found := store.Check(id); !found || len(actions) != 1 || actions[0] != ApprovalActionRead {
		t.Fatalf("expected ([read],true) after saving allow decision, got (%v,%v)", actions, found)
	}

	// Now construct a second store pointing at the same file and ensure
	// it reads the persisted data.
	store2, err := NewJSONApprovalPolicyStore(path)
	if err != nil {
		t.Fatalf("NewJSONApprovalPolicyStore(%q) second instance error = %v, want nil", path, err)
	}

	if actions, found := store2.Check(id); !found || len(actions) != 1 || actions[0] != ApprovalActionRead {
		t.Fatalf("expected persisted policy to be visible to new store, got (%v,%v)", actions, found)
	}
}

func TestJSONApprovalPolicyStore_DirectoryRecursion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policies.json")

	store, err := NewJSONApprovalPolicyStore(path)
	if err != nil {
		t.Fatalf("NewJSONApprovalPolicyStore(%q) error = %v, want nil", path, err)
	}

	baseDir := filepath.Join("root", "dir")
	subDir := filepath.Join(baseDir, "subdir")

	baseID := ApprovalPolicyID(ApprovalSubsysTools, ApprovalGroupFileIO, ApprovalTargetDir, baseDir)
	subID := ApprovalPolicyID(ApprovalSubsysTools, ApprovalGroupFileIO, ApprovalTargetDir, subDir)

	store.Save(baseID, []ApprovalAction{ApprovalActionRead})

	if actions, found := store.Check(subID); !found || len(actions) != 1 || actions[0] != ApprovalActionRead {
		t.Fatalf("expected recursive allow for subdirectory, got (%v,%v)", actions, found)
	}
}

func TestJSONApprovalPolicyStore_DirectoryMostSpecific(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policies.json")

	store, err := NewJSONApprovalPolicyStore(path)
	if err != nil {
		t.Fatalf("NewJSONApprovalPolicyStore(%q) error = %v, want nil", path, err)
	}

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

func TestJSONApprovalPolicyStore_DirectoryDifferentSubsysOrGroup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policies.json")

	store, err := NewJSONApprovalPolicyStore(path)
	if err != nil {
		t.Fatalf("NewJSONApprovalPolicyStore(%q) error = %v, want nil", path, err)
	}

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

func TestJSONApprovalPolicyStore_DirectoryNotAppliedToFileTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policies.json")

	store, err := NewJSONApprovalPolicyStore(path)
	if err != nil {
		t.Fatalf("NewJSONApprovalPolicyStore(%q) error = %v, want nil", path, err)
	}

	baseDir := filepath.Join("root", "dir")
	subDir := filepath.Join(baseDir, "subdir")

	dirID := ApprovalPolicyID(ApprovalSubsysTools, ApprovalGroupFileIO, ApprovalTargetDir, baseDir)
	store.Save(dirID, []ApprovalAction{ApprovalActionRead})

	fileID := ApprovalPolicyID(ApprovalSubsysTools, ApprovalGroupFileIO, ApprovalTargetFile, subDir)

	if actions, found := store.Check(fileID); len(actions) != 0 || found {
		t.Fatalf("expected directory policy not to apply to file target, got (%v,%v)", actions, found)
	}
}
