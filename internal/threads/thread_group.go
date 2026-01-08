/* Copyright © 2023-2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package threads

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
	"unsafe"

	"github.com/google/uuid"

	"github.com/mikeb26/gptcli/internal/prompts"
	"github.com/mikeb26/gptcli/internal/types"
)

type ThreadGroup struct {
	prefix       string
	threads      []*thread
	totThreads   int
	dir          string
	curThreadNum int
	mu           sync.RWMutex
}

func NewThreadGroup(prefixIn string, dirIn string) *ThreadGroup {

	thrGrp := &ThreadGroup{
		prefix:       prefixIn,
		threads:      make([]*thread, 0),
		totThreads:   0,
		dir:          dirIn,
		curThreadNum: 0,
	}

	return thrGrp
}

func (thrGrp *ThreadGroup) Threads() []Thread {
	thrGrp.mu.RLock()
	defer thrGrp.mu.RUnlock()

	out := make([]Thread, 0, len(thrGrp.threads))
	for _, thr := range thrGrp.threads {
		out = append(out, thr)
	}

	return out
}

func (thrGrp *ThreadGroup) Prefix() string {
	thrGrp.mu.RLock()
	defer thrGrp.mu.RUnlock()

	return thrGrp.prefix
}

// NonIdleThreadCount returns the number of threads in the group that are not
// idle (e.g. running or blocked waiting for user approval).
//
// This is intended for UI layers that want to warn the user before quitting.
func (thrGrp *ThreadGroup) NonIdleThreadCount() int {
	thrGrp.mu.RLock()
	defer thrGrp.mu.RUnlock()

	count := 0
	for _, thr := range thrGrp.threads {
		if thr == nil {
			continue
		}
		if thr.State() != ThreadStateIdle {
			count++
		}
	}

	return count
}

func (thrGrp *ThreadGroup) hasNonIdleThreads() bool {
	// caller holds thrGrp.mu so each thread's state cannot transition
	// out of idle; see setCurrentThreadRunning()
	for _, thr := range thrGrp.threads {
		if thr.State() != ThreadStateIdle {
			return true
		}
	}

	return false
}

func (thrGrp *ThreadGroup) LoadThreads() error {
	thrGrp.mu.Lock()
	defer thrGrp.mu.Unlock()

	if thrGrp.hasNonIdleThreads() {
		return fmt.Errorf("Cannot re-load thread group with non-idle threads")
	}

	thrGrp.curThreadNum = 0
	thrGrp.totThreads = 0
	thrGrp.threads = make([]*thread, 0)

	dEntries, err := os.ReadDir(thrGrp.dir)
	if err != nil {
		return fmt.Errorf("Failed to read dir %v: %w", thrGrp.dir, err)
	}

	for _, dEnt := range dEntries {
		fullpath := filepath.Join(thrGrp.dir, dEnt.Name())
		threadFileText, err := os.ReadFile(fullpath)
		if err != nil {
			return fmt.Errorf("Failed to read %v: %w", fullpath, err)
		}

		var thread thread
		err = json.Unmarshal(threadFileText, &thread.persisted)
		if err != nil {
			return fmt.Errorf("Failed to parse %v: %w", fullpath, err)
		}
		if thread.persisted.Id == "" {
			thread.persisted.Id = uuid.NewString()
		}
		thread.state = ThreadStateIdle
		thread.fileName = genUniqFileName(thread.persisted.Name,
			thread.persisted.CreateTime)
		if thread.fileName != dEnt.Name() {
			oldPath := filepath.Join(thrGrp.dir, dEnt.Name())
			newPath := filepath.Join(thrGrp.dir, thread.fileName)
			fmt.Fprintf(os.Stderr, "Renaming thread %v to %v\n",
				oldPath, newPath)
			_ = os.Remove(oldPath)
			_ = thread.save(thrGrp.dir)
		}

		_ = thrGrp.addThread(&thread)
	}

	return nil
}

// activateThread updates the thread group's current thread state,
// refreshes the access time, and persists the thread to disk. It
// performs no user-facing I/O and is therefore safe to call from
// different UIs (CLI, ncurses, etc.).
func (thrGrp *ThreadGroup) ActivateThread(threadNum int) (Thread, error) {
	thrGrp.mu.Lock()
	defer thrGrp.mu.Unlock()

	if threadNum > thrGrp.totThreads || threadNum == 0 {
		threadNumPrint := fmt.Sprintf("%v%v", thrGrp.prefix, threadNum)
		return nil, fmt.Errorf(ThreadNoExistErrFmt, threadNumPrint)
	}

	thrGrp.curThreadNum = threadNum
	thread := thrGrp.threads[thrGrp.curThreadNum-1]

	thread.mu.Lock()
	defer thread.mu.Unlock()

	thread.persisted.AccessTime = time.Now()
	// Persisting a running/blocked thread would fail (and is generally not
	// desirable) because the thread may be actively mutating in the background.
	//
	// Allow UIs to "activate" (focus) a non-idle thread so they can
	// detach/reattach to a running thread without failing here.
	if thread.state == ThreadStateIdle {
		if err := thread.save(thrGrp.dir); err != nil {
			return nil, err
		}
	}

	return thread, nil
}

// NewThread encapsulates the logic to allocate and register a new
// thread in the main thread group. It is used both by the CLI "new"
// subcommand and the ncurses menu UI so their behavior stays in sync.
func (thrGrp *ThreadGroup) NewThread(name string) error {
	thrGrp.mu.Lock()
	defer thrGrp.mu.Unlock()

	cTime := time.Now()
	fileName := genUniqFileName(name, cTime)

	dialogue := []*types.ThreadMessage{
		{Role: types.LlmRoleSystem,
			Content: prompts.SystemMsg},
	}

	curThread := &thread{
		persisted: persistedThread{
			Name:       name,
			CreateTime: cTime,
			AccessTime: cTime,
			ModTime:    cTime,
			Dialogue:   dialogue,
			Id:         uuid.NewString(),
		},
		fileName: fileName,
		state:    ThreadStateIdle,
	}

	thrGrp.curThreadNum = thrGrp.addThread(curThread)

	return nil
}

func (thrGrp *ThreadGroup) addThread(curThread *thread) int {
	thrGrp.totThreads++
	thrGrp.threads = append(thrGrp.threads, curThread)

	return thrGrp.totThreads
}

// @todo need ux
//  unarchiveThreadMain()

func (thrGrp *ThreadGroup) Count() int {
	thrGrp.mu.RLock()
	defer thrGrp.mu.RUnlock()

	return thrGrp.totThreads
}

func (srcThrGrp *ThreadGroup) MoveThread(threadNum int,
	dstThrGrp *ThreadGroup) error {
	// Ensure consistent locking order to prevent deadlocks if two goroutines
	// concurrently move threads in opposite directions between two groups.
	first, second := srcThrGrp, dstThrGrp
	if uintptr(unsafe.Pointer(first)) > uintptr(unsafe.Pointer(second)) {
		first, second = second, first
	}

	first.mu.Lock()
	defer first.mu.Unlock()
	second.mu.Lock()
	defer second.mu.Unlock()

	if threadNum > srcThrGrp.totThreads || threadNum == 0 {
		threadNumPrint := fmt.Sprintf("%v%v", srcThrGrp.prefix, threadNum)
		return fmt.Errorf(ThreadNoExistErrFmt, threadNumPrint)
	}

	thread := srcThrGrp.threads[threadNum-1]

	thread.mu.Lock()
	defer thread.mu.Unlock()

	err := thread.save(dstThrGrp.dir)
	if err != nil {
		return err
	}
	err = thread.remove(srcThrGrp.dir)
	if err != nil {
		_ = thread.remove(dstThrGrp.dir)
		return err
	}
	srcThrGrp.curThreadNum = 0

	dstThrGrp.addThread(thread)

	srcThrGrp.totThreads--
	srcThrGrp.threads = slices.Delete(srcThrGrp.threads, threadNum-1, threadNum)

	return nil
}

// ThreadId returns the thread id of the specified thread number in the group
func (thrGrp *ThreadGroup) ThreadId(threadNum int) string {
	thrGrp.mu.RLock()
	defer thrGrp.mu.RUnlock()

	if threadNum > thrGrp.totThreads || threadNum == 0 {
		return ""
	}
	thread := thrGrp.threads[threadNum-1]

	return thread.Id()
}
