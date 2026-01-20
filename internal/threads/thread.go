/* Copyright © 2023-2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package threads

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mikeb26/gptcli/internal/types"
)

const (
	ThreadNoExistErrFmt = "Thread %v does not exist. To list threads try 'ls'.\n"
	ThreadFileName      = "thread.json"
)

type ThreadState int

const (
	ThreadStateUnknown ThreadState = iota

	ThreadStateIdle
	ThreadStateRunning
	ThreadStateBlocked // e.g. waiting for user approval

	ThreadStateInvalid ThreadState = 2147483647
)

func (state ThreadState) String() string {
	switch state {
	case ThreadStateIdle:
		return "idle"
	case ThreadStateRunning:
		return "running"
	case ThreadStateBlocked:
		return "blocked"
	default:
	}

	return fmt.Sprintf("invalid <%v>", int(state))
}

type persistedThread struct {
	Name            string                 `json:"name"`
	CreateTime      time.Time              `json:"ctime"`
	AccessTime      time.Time              `json:"atime"`
	ModTime         time.Time              `json:"mtime"`
	Dialogue        []*types.ThreadMessage `json:"dialogue"`
	Id              string                 `json:"id3"`
	InvocationCount int                    `json:"inv_count"`
}

type Thread interface {
	State() ThreadState
	Id() string
	Name() string
	CreateTime() time.Time
	AccessTime() time.Time
	ModTime() time.Time
	Dialogue() []*types.ThreadMessage
	RenderBlocks() []RenderBlock
	Access() error
	ChatOnceAsync(context.Context, types.InternalContext, string,
		bool, string) (*RunningThreadState, error)
}

type thread struct {
	persisted persistedThread

	dirName   string
	parentDir string
	state     ThreadState
	runState  *RunningThreadState
	parent    *ThreadGroup

	// llmClient is created per-thread (and may be recreated as needed).
	llmClient types.AIClient
	// asyncApprover is per-thread and is used to route approvals back to the UI
	// goroutine servicing this thread.
	asyncApprover *AsyncApprover
	mu            sync.RWMutex
}

// load restores a thread from disk.
//
// It also normalizes legacy/stale filenames by renaming the persisted
// thread file to match the current genUniqFileName scheme.
func (t *thread) load(parentDir string, dirName string) error {
	fullpath := filepath.Join(parentDir, dirName, ThreadFileName)
	threadFileText, err := os.ReadFile(fullpath)
	if err != nil {
		return fmt.Errorf("Failed to read %v: %w", fullpath, err)
	}

	if err := json.Unmarshal(threadFileText, &t.persisted); err != nil {
		return fmt.Errorf("Failed to parse %v: %w", fullpath, err)
	}

	t.state = ThreadStateIdle
	t.parentDir = parentDir
	t.dirName = dirName

	if t.persisted.Id == "" {
		id, err := t.parent.parent.newThreadId()
		if err != nil {
			return err
		}
		t.persisted.Id = id
		// best effort save w/ new thread id
		_ = t.save()
	}

	return nil
}

// State returns the current thread state. It is primarily intended for UI
// layers that want to render state (running/blocked/etc.).
func (t *thread) State() ThreadState {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.state
}

// SetState sets the current thread state.
func (t *thread) setState(state ThreadState) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.state = state
}

// Id returns the current thread id
func (t *thread) Id() string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.persisted.Id
}

// CreateTime returns the thread creation timestamp.
func (t *thread) CreateTime() time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.persisted.CreateTime
}

// AccessTime returns the last access timestamp.
func (t *thread) AccessTime() time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.persisted.AccessTime
}

// ModTime returns the last modified timestamp.
func (t *thread) ModTime() time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.persisted.ModTime
}

// Dialogue returns a deep copy of the thread's dialogue
func (t *thread) Dialogue() []*types.ThreadMessage {
	t.mu.RLock()
	defer t.mu.RUnlock()

	orig := t.persisted.Dialogue
	dCopy := make([]*types.ThreadMessage, len(orig))
	copy(dCopy, orig)

	return dCopy
}

// Name returns the thread's name
func (t *thread) Name() string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.persisted.Name
}

// save persists the thread's dialogue to a file; callers should already hold
// a write lock on the thread's mutex
func (t *thread) save() error {
	return t.saveWithDir(t.parentDir)
}
func (t *thread) saveWithDir(parentDir string) error {
	if t.state != ThreadStateIdle {
		return fmt.Errorf("cannot save non-idle thread state:%v", t.state)
	}

	threadFileContent, err := json.Marshal(&t.persisted)
	if err != nil {
		return fmt.Errorf("Failed to save thread %v: %w", t.persisted.Name,
			err)
	}

	threadDir := filepath.Join(parentDir, t.dirName)
	err = os.MkdirAll(threadDir, 0700)
	if err != nil {
		return fmt.Errorf("Failed to save thread %v: %w", t.persisted.Name,
			err)
	}

	filePath := filepath.Join(threadDir, ThreadFileName)
	err = os.WriteFile(filePath, threadFileContent, 0600)
	if err != nil {
		return fmt.Errorf("Failed to save thread %v(%v): %w",
			t.persisted.Name, filePath, err)
	}

	return nil
}

// remove deletes the thread's persisted dialogue; callers should already hold
// a write lock on the thread's mutex
func (t *thread) remove() error {
	return t.removeWithDir(t.parentDir)
}
func (t *thread) removeWithDir(parentDir string) error {
	if t.state != ThreadStateIdle {
		return fmt.Errorf("cannot remove non-idle thread state:%v",
			t.state)
	}

	threadDir := filepath.Join(parentDir, t.dirName)
	err := os.RemoveAll(threadDir)
	if err != nil {
		return fmt.Errorf("Failed to delete thread %v(%v): %w",
			t.persisted.Name, threadDir, err)
	}

	return nil
}
