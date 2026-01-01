/* Copyright © 2023-2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package threads

import (
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
	RowFmt              = "│ %8v │ %8v │ %18v │ %18v │ %18v │ %-18v\n"
	RowSpacer           = "──────────────────────────────────────────────────────────────────────────────────────────────\n"
)

type GptCliThreadState int

const (
	GptCliThreadStateUnknown GptCliThreadState = iota

	GptCliThreadStateIdle
	GptCliThreadStateRunning
	GptCliThreadStateBlocked // e.g. waiting for user approval

	GptCliThreadStateInvalid GptCliThreadState = 2147483647
)

func (state GptCliThreadState) String() string {
	switch state {
	case GptCliThreadStateIdle:
		return "idle"
	case GptCliThreadStateRunning:
		return "running"
	case GptCliThreadStateBlocked:
		return "blocked"
	default:
	}

	return fmt.Sprintf("invalid <%v>", int(state))
}

type persistedThread struct {
	Name       string                 `json:"name"`
	CreateTime time.Time              `json:"ctime"`
	AccessTime time.Time              `json:"atime"`
	ModTime    time.Time              `json:"mtime"`
	Dialogue   []*types.GptCliMessage `json:"dialogue"`
}

type Thread struct {
	persisted persistedThread

	fileName string
	state    GptCliThreadState
	mu       sync.RWMutex
}

// State returns the current thread state. It is primarily intended for UI
// layers that want to render state (running/blocked/etc.).
func (thread *Thread) State() GptCliThreadState {
	thread.mu.RLock()
	defer thread.mu.RUnlock()

	return thread.state
}

// SetState sets the current thread state.
func (thread *Thread) SetState(state GptCliThreadState) {
	thread.mu.Lock()
	defer thread.mu.Unlock()

	thread.state = state
}

// Dialogue returns a deep copy of the thread's dialogue
func (thread *Thread) Dialogue() []*types.GptCliMessage {
	thread.mu.RLock()
	defer thread.mu.RUnlock()

	orig := thread.persisted.Dialogue
	dCopy := make([]*types.GptCliMessage, len(orig))
	copy(dCopy, orig)

	return dCopy
}

// AppendDialogue appends a message to the existing thred dialogue
func (thread *Thread) AppendDialogue(msg *types.GptCliMessage) {
	thread.mu.Lock()
	defer thread.mu.Unlock()

	thread.persisted.Dialogue = append(thread.persisted.Dialogue, msg)
}

// Name returns the thread's name
func (thread *Thread) Name() string {
	thread.mu.RLock()
	defer thread.mu.RUnlock()

	return thread.persisted.Name
}

// Copy returns a deep copy of the thread
func (thread *Thread) Copy() *Thread {
	thread.mu.RLock()
	defer thread.mu.RUnlock()

	return thread.copyInt()
}

func (thread *Thread) copyInt() *Thread {
	var thrCopy Thread
	thrCopy = *thread
	thrCopy.mu = sync.RWMutex{}
	thrCopy.state = GptCliThreadStateIdle
	orig := thread.persisted.Dialogue
	dCopy := make([]*types.GptCliMessage, len(orig))
	copy(dCopy, orig)
	thrCopy.persisted.Dialogue = dCopy

	return &thrCopy
}

// save persists the thread's dialogue to a file; callers should already hold
// a write lock on the thread's mutex
func (thread *Thread) save(dir string) error {
	if thread.state != GptCliThreadStateIdle {
		return fmt.Errorf("cannot save non-idle thread state:%v", thread.state)
	}

	threadFileContent, err := json.Marshal(&thread.persisted)
	if err != nil {
		return fmt.Errorf("Failed to save thread %v: %w", thread.persisted.Name,
			err)
	}

	filePath := filepath.Join(dir, thread.fileName)
	err = os.WriteFile(filePath, threadFileContent, 0600)
	if err != nil {
		return fmt.Errorf("Failed to save thread %v(%v): %w",
			thread.persisted.Name, filePath, err)
	}

	return nil
}

// remove deletes the thread's persisted dialogue; callers should already hold
// a write lock on the thread's mutex
func (thread *Thread) remove(dir string) error {
	if thread.state != GptCliThreadStateIdle {
		return fmt.Errorf("cannot remove non-idle thread state:%v",
			thread.state)
	}

	filePath := filepath.Join(dir, thread.fileName)
	err := os.Remove(filePath)
	if err != nil {
		return fmt.Errorf("Failed to delete thread %v(%v): %w",
			thread.persisted.Name, filePath, err)
	}

	return nil
}

func (t *Thread) HeaderString(threadNum string) string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	now := time.Now()

	cTime := formatHeaderTime(t.persisted.CreateTime, now)
	aTime := formatHeaderTime(t.persisted.AccessTime, now)
	mTime := formatHeaderTime(t.persisted.ModTime, now)

	return fmt.Sprintf(RowFmt, threadNum, t.state, aTime, mTime, cTime,
		t.persisted.Name)
}
