/* Copyright © 2023-2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package threads

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestThreadGroupSet_Save(t *testing.T) {
	root := t.TempDir()

	tgs := NewThreadGroupSet(root, nil)

	tgs.mu.Lock()
	err := tgs.save()
	tgs.mu.Unlock()
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}

	path := filepath.Join(root, threadGroupSetFileName)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file %q to exist: %v", path, err)
	}
}

func TestThreadGroupSet_Load_MissingFile(t *testing.T) {
	root := t.TempDir()

	tgs := NewThreadGroupSet(root, nil)
	if err := tgs.Load(); err != nil {
		t.Fatalf("load failed: %v", err)
	}

	// Defaults preserved.
	if tgs.persisted.ThreadNum != 0 {
		t.Fatalf("expected ThreadNum=0, got %v", tgs.persisted.ThreadNum)
	}
}

func TestThreadGroupSet_Load_RestoresFields(t *testing.T) {
	root := t.TempDir()

	// Write a persisted file with a non-default thread num.
	content, err := json.Marshal(&persistedThreadGroupSet{ThreadNum: 42})
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	path := filepath.Join(root, threadGroupSetFileName)
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	tgs := NewThreadGroupSet(root, nil)
	if err := tgs.Load(); err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if tgs.persisted.ThreadNum != 42 {
		t.Fatalf("expected ThreadNum=42, got %v", tgs.persisted.ThreadNum)
	}
}

func TestThreadGroupSet_NewThreadGroupSet_PrefixRules(t *testing.T) {
	root := t.TempDir()

	// NewThreadGroupSet no longer returns an error and no longer enforces
	// prefix validation. Ensure it constructs groups as requested.
	tgs := NewThreadGroupSet(root, []string{""})
	if len(tgs.threadGrps) != 1 {
		t.Fatalf("expected 1 group, got %v", len(tgs.threadGrps))
	}
	if tgs.threadGrps[0].Name() != "" {
		t.Fatalf("expected group name to remain empty string, got %q", tgs.threadGrps[0].Name())
	}
}
