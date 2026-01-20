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

func TestThreadGroupSet_NonIdleThreadCount_EmptySet(t *testing.T) {
	root := t.TempDir()

	tgs := NewThreadGroupSet(root, nil)
	if got := tgs.NonIdleThreadCount(); got != 0 {
		t.Fatalf("expected NonIdleThreadCount=0, got %v", got)
	}
}

func TestThreadGroupSet_NonIdleThreadCount_SumsAcrossGroups(t *testing.T) {
	root := t.TempDir()

	tgs := NewThreadGroupSet(root, []string{"grpA", "grpB"})

	// Create some threads; new threads start idle.
	if err := tgs.NewThread("grpA", "t1"); err != nil {
		t.Fatalf("NewThread failed: %v", err)
	}
	if err := tgs.NewThread("grpA", "t2"); err != nil {
		t.Fatalf("NewThread failed: %v", err)
	}
	if err := tgs.NewThread("grpB", "t3"); err != nil {
		t.Fatalf("NewThread failed: %v", err)
	}

	if got := tgs.NonIdleThreadCount(); got != 0 {
		t.Fatalf("expected NonIdleThreadCount=0 for all-idle threads, got %v", got)
	}

	// Mark one thread in grpA as running and one thread in grpB as blocked.
	// (Use the underlying map to avoid relying on sort order.)
	grpA := tgs.threadGrps[0]
	grpA.mu.RLock()
	if len(grpA.threads) != 2 {
		grpA.mu.RUnlock()
		t.Fatalf("expected 2 threads in grpA, got %v", len(grpA.threads))
	}
	for _, thr := range grpA.threads {
		thr.setState(ThreadStateRunning)
		break
	}
	grpA.mu.RUnlock()

	grpB := tgs.threadGrps[1]
	grpB.mu.RLock()
	if len(grpB.threads) != 1 {
		grpB.mu.RUnlock()
		t.Fatalf("expected 1 thread in grpB, got %v", len(grpB.threads))
	}
	for _, thr := range grpB.threads {
		thr.setState(ThreadStateBlocked)
		break
	}
	grpB.mu.RUnlock()

	if got := tgs.NonIdleThreadCount(); got != 2 {
		t.Fatalf("expected NonIdleThreadCount=2, got %v", got)
	}
}
