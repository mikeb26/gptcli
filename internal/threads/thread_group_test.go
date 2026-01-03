package threads

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mikeb26/gptcli/internal/prompts"
	"github.com/mikeb26/gptcli/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestNewThreadInitializesAndRegistersThread(t *testing.T) {
	dir := t.TempDir()

	grp := NewThreadGroup("T", dir)
	err := grp.NewThread("first-thread")
	assert.NoError(t, err)

	// Thread is registered in-memory.
	assert.Equal(t, 1, grp.Count())
	threads := grp.Threads()
	if assert.Len(t, threads, 1) {
		thr := threads[0]
		assert.Equal(t, "first-thread", thr.Name())
		// All timestamps are initialized to the same creation time.
		assert.True(t, thr.persisted.CreateTime.Equal(thr.persisted.AccessTime))
		assert.True(t, thr.persisted.CreateTime.Equal(thr.persisted.ModTime))
		assert.NotEmpty(t, thr.fileName)

		// Initial dialogue contains only the system message.
		d := thr.Dialogue()
		if assert.Len(t, d, 1) {
			msg := d[0]
			assert.Equal(t, types.GptCliMessageRoleSystem, msg.Role)
			assert.Equal(t, prompts.SystemMsg, msg.Content)
		}
	}

	// NewThread does not persist to disk by itself.
	entries, err := os.ReadDir(dir)
	assert.NoError(t, err)
	assert.Len(t, entries, 0)
}

func TestActivateThreadUpdatesAccessTimeAndPersists(t *testing.T) {
	dir := t.TempDir()

	grp := NewThreadGroup("T", dir)

	base := time.Now().Add(-time.Hour)
	thr := &Thread{persisted: persistedThread{
		Name:       "activate-me",
		CreateTime: base,
		AccessTime: base,
		ModTime:    base,
		Dialogue:   []*types.GptCliMessage{},
	}}
	thr.state = ThreadStateIdle
	thr.fileName = genUniqFileName(thr.persisted.Name, thr.persisted.CreateTime)
	// Persist initial state so ActivateThread can overwrite it.
	assert.NoError(t, thr.save(dir))

	grp.addThread(thr)
	oldAccess := thr.persisted.AccessTime

	activated, err := grp.ActivateThread(1)
	assert.NoError(t, err)
	assert.Equal(t, thr, activated)
	assert.Equal(t, 1, grp.curThreadNum)
	assert.True(t, activated.persisted.AccessTime.After(oldAccess))

	// Verify the on-disk representation has the updated access time.
	data, err := os.ReadFile(filepath.Join(dir, thr.fileName))
	assert.NoError(t, err)

	var diskThread Thread
	assert.NoError(t, json.Unmarshal(data, &diskThread.persisted))
	assert.True(t, diskThread.persisted.AccessTime.After(oldAccess))
}

func TestActivateThreadInvalidIndex(t *testing.T) {
	dir := t.TempDir()
	grp := NewThreadGroup("T", dir)

	thr, err := grp.ActivateThread(1)
	assert.Nil(t, thr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Thread T1")

	thr, err = grp.ActivateThread(0)
	assert.Nil(t, thr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Thread T0")
}

func TestLoadThreadsLoadsAndRenamesStaleFiles(t *testing.T) {
	dir := t.TempDir()

	// Create a thread JSON with a stale filename that does not match
	// the genUniqFileName scheme so LoadThreads will rename it.
	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	orig := &Thread{persisted: persistedThread{
		Name:       "rename-thread",
		CreateTime: base,
		AccessTime: base,
		ModTime:    base,
		Dialogue:   []*types.GptCliMessage{},
	}}
	orig.state = ThreadStateIdle

	data, err := json.Marshal(orig.persisted)
	assert.NoError(t, err)

	staleName := "stale-name.json"
	stalePath := filepath.Join(dir, staleName)
	assert.NoError(t, os.WriteFile(stalePath, data, 0600))

	grp := NewThreadGroup("T", dir)
	assert.NoError(t, grp.LoadThreads())

	threads := grp.Threads()
	if assert.Len(t, threads, 1) {
		loaded := threads[0]
		expectedFileName := genUniqFileName(loaded.persisted.Name, loaded.persisted.CreateTime)
		assert.Equal(t, expectedFileName, loaded.fileName)

		// Old file should be gone; new one should exist.
		_, err = os.Stat(stalePath)
		assert.Error(t, err)
		assert.True(t, os.IsNotExist(err))

		_, err = os.Stat(filepath.Join(dir, expectedFileName))
		assert.NoError(t, err)
	}
}

func TestMoveThreadMovesFileAndReloadsSourceGroup(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	srcGrp := NewThreadGroup("S", srcDir)
	dstGrp := NewThreadGroup("D", dstDir)

	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	thr := &Thread{persisted: persistedThread{
		Name:       "move-me",
		CreateTime: base,
		AccessTime: base,
		ModTime:    base,
		Dialogue:   []*types.GptCliMessage{},
	}}
	thr.state = ThreadStateIdle
	thr.fileName = genUniqFileName(thr.persisted.Name, thr.persisted.CreateTime)
	assert.NoError(t, thr.save(srcDir))

	srcGrp.addThread(thr)
	assert.Equal(t, 1, srcGrp.Count())

	// Move the thread from src to dst.
	err := srcGrp.MoveThread(1, dstGrp)
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
	_, err = os.Stat(filepath.Join(srcDir, thr.fileName))
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))

	_, err = os.Stat(filepath.Join(dstDir, thr.fileName))
	assert.NoError(t, err)
}

func TestMoveThreadInvalidIndex(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	srcGrp := NewThreadGroup("S", srcDir)
	dstGrp := NewThreadGroup("D", dstDir)

	err := srcGrp.MoveThread(1, dstGrp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Thread S1")
}
