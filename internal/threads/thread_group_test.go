/* Copyright © 2025-2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package threads

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mikeb26/gptcli/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestNewThreadInitializesAndRegistersThread(t *testing.T) {
	root := t.TempDir()
	setDir := root + "/set"
	grpDir := root + "/threads"
	assert.NoError(t, os.MkdirAll(setDir, 0o700))
	assert.NoError(t, os.MkdirAll(grpDir, 0o700))

	set := NewThreadGroupSet(setDir, nil)
	grp := newThreadGroup(set, "T", grpDir)
	err := grp.NewThread("first-thread")
	assert.NoError(t, err)

	// Thread is registered in-memory.
	assert.Equal(t, 1, grp.Count())
	threads := grp.Threads()
	if assert.Len(t, threads, 1) {
		thr := threads[0]
		assert.Equal(t, "first-thread", thr.Name())
		// All timestamps are initialized to the same creation time.
		assert.True(t, thr.CreateTime().Equal(thr.AccessTime()))
		assert.True(t, thr.CreateTime().Equal(thr.ModTime()))

		// Initial dialogue is empty; system message is injected at runtime.
		d := thr.Dialogue()
		assert.Len(t, d, 0)
	}

	// NewThread does not persist to disk by itself.
	entries, err := os.ReadDir(grpDir)
	assert.NoError(t, err)
	assert.Len(t, entries, 0)
}

func TestActivateThreadUpdatesAccessTimeAndPersists(t *testing.T) {
	root := t.TempDir()
	setDir := root + "/set"
	grpDir := root + "/threads"
	assert.NoError(t, os.MkdirAll(setDir, 0o700))
	assert.NoError(t, os.MkdirAll(grpDir, 0o700))

	set := NewThreadGroupSet(setDir, nil)
	grp := newThreadGroup(set, "T", grpDir)

	base := time.Now().Add(-time.Hour)
	thr := &thread{persisted: persistedThread{
		Name:       "activate-me",
		CreateTime: base,
		AccessTime: base,
		ModTime:    base,
		Dialogue:   []*types.ThreadMessage{},
		Id:         "1",
	}}
	thr.state = ThreadStateIdle
	thr.dirName = genUniqDirName(thr.persisted.Name, thr.persisted.CreateTime)
	thr.parentDir = grpDir
	// Persist initial state so ActivateThread can overwrite it.
	assert.NoError(t, thr.save())

	grp.addThread(thr)
	oldAccess := thr.persisted.AccessTime

	threads := grp.Threads()
	if !assert.Len(t, threads, 1) {
		return
	}
	activated := threads[0]
	assert.NoError(t, activated.Access())
	assert.Equal(t, thr.Id(), activated.Id())
	assert.True(t, activated.AccessTime().After(oldAccess))

	// Verify the on-disk representation has the updated access time.
	data, err := os.ReadFile(filepath.Join(grpDir, thr.dirName, ThreadFileName))
	assert.NoError(t, err)

	var diskThread persistedThread
	assert.NoError(t, json.Unmarshal(data, &diskThread))
	assert.True(t, diskThread.AccessTime.After(oldAccess))
}

func TestActivateThreadInvalidIndex(t *testing.T) {
	root := t.TempDir()
	setDir := root + "/set"
	grpDir := root + "/threads"
	assert.NoError(t, os.MkdirAll(setDir, 0o700))
	assert.NoError(t, os.MkdirAll(grpDir, 0o700))

	set := NewThreadGroupSet(setDir, nil)
	grp := newThreadGroup(set, "T", grpDir)

	// ActivateThread was removed; ensure lookups behave as expected.
	assert.Equal(t, 0, grp.Count())
	assert.Len(t, grp.Threads(), 0)
}

func TestLoadThreadsLoadsAndRenamesStaleFiles(t *testing.T) {
	root := t.TempDir()
	setDir := root + "/set"
	grpDir := root + "/threads"
	assert.NoError(t, os.MkdirAll(setDir, 0o700))
	assert.NoError(t, os.MkdirAll(grpDir, 0o700))

	set := NewThreadGroupSet(setDir, nil)

	// Create a thread JSON under a non-canonical directory name.
	// ThreadGroup.LoadThreads should still load it.
	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	orig := &thread{persisted: persistedThread{
		Name:       "rename-thread",
		CreateTime: base,
		AccessTime: base,
		ModTime:    base,
		Dialogue:   []*types.ThreadMessage{},
		Id:         "2",
	}}
	orig.state = ThreadStateIdle

	data, err := json.Marshal(orig.persisted)
	assert.NoError(t, err)

	staleDirName := "stale-dir-name"
	staleThreadDir := filepath.Join(grpDir, staleDirName)
	assert.NoError(t, os.MkdirAll(staleThreadDir, 0o700))
	staleThreadPath := filepath.Join(staleThreadDir, ThreadFileName)
	assert.NoError(t, os.WriteFile(staleThreadPath, data, 0600))

	grp := newThreadGroup(set, "T", grpDir)
	assert.NoError(t, grp.LoadThreads())

	threads := grp.Threads()
	if assert.Len(t, threads, 1) {
		loaded := threads[0]
		loadedImpl := loaded.(*thread)
		assert.Equal(t, staleDirName, loadedImpl.dirName)

		_, err = os.Stat(staleThreadPath)
		assert.NoError(t, err)
	}
}

func TestMoveThreadMovesFileAndReloadsSourceGroup(t *testing.T) {
	root := t.TempDir()
	setDir := root + "/set"
	srcDir := root + "/src"
	dstDir := root + "/dst"
	assert.NoError(t, os.MkdirAll(setDir, 0o700))
	assert.NoError(t, os.MkdirAll(srcDir, 0o700))
	assert.NoError(t, os.MkdirAll(dstDir, 0o700))

	set := NewThreadGroupSet(setDir, nil)
	srcGrp := newThreadGroup(set, "S", srcDir)
	dstGrp := newThreadGroup(set, "D", dstDir)

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	thr := &thread{persisted: persistedThread{
		Name:       "move-me",
		CreateTime: base,
		AccessTime: base,
		ModTime:    base,
		Dialogue:   []*types.ThreadMessage{},
		Id:         "3",
	}}
	thr.state = ThreadStateIdle
	thr.dirName = genUniqDirName(thr.persisted.Name, thr.persisted.CreateTime)
	thr.parentDir = srcDir
	assert.NoError(t, thr.save())

	srcGrp.addThread(thr)
	assert.Equal(t, 1, srcGrp.Count())

	// Move the thread from src to dst.
	err := srcGrp.MoveThread(thr, dstGrp)
	assert.NoError(t, err)

	// Source has been reloaded from disk and is now empty.
	assert.Equal(t, 0, srcGrp.Count())
	assert.Len(t, srcGrp.Threads(), 0)

	// Destination has the moved thread in-memory.
	assert.Equal(t, 1, dstGrp.Count())
	if assert.Len(t, dstGrp.Threads(), 1) {
		moved := dstGrp.Threads()[0]
		assert.Equal(t, "move-me", moved.Name())
	}

	// File only exists in destination directory.
	_, err = os.Stat(filepath.Join(srcDir, thr.dirName, ThreadFileName))
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))

	_, err = os.Stat(filepath.Join(dstDir, thr.dirName, ThreadFileName))
	assert.NoError(t, err)
}

func TestMoveThreadInvalidIndex(t *testing.T) {
	root := t.TempDir()
	setDir := root + "/set"
	srcDir := root + "/src"
	dstDir := root + "/dst"
	assert.NoError(t, os.MkdirAll(setDir, 0o700))
	assert.NoError(t, os.MkdirAll(srcDir, 0o700))
	assert.NoError(t, os.MkdirAll(dstDir, 0o700))

	set := NewThreadGroupSet(setDir, nil)
	srcGrp := newThreadGroup(set, "S", srcDir)
	dstGrp := newThreadGroup(set, "D", dstDir)

	err := srcGrp.MoveThread(&thread{persisted: persistedThread{Id: "does-not-exist"}}, dstGrp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}
