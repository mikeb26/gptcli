/* Copyright © 2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package threads

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
)

const (
	threadGroupSetFileName = "thread_group_set.json"
)

type persistedThreadGroupSet struct {
	ThreadNum int64 `json:"thread_num"`
}

// ThreadGroupSet is a concurrency-safe container for 0 or more ThreadGroups.
//
// It also owns persisted metadata that is shared across its ThreadGroups.
//
// NOTE: The persistence file is written directly under dir, so the
// ThreadGroupSet dir MUST NOT be the same directory as any ThreadGroup dir
// that contains thread JSON files.
type ThreadGroupSet struct {
	persisted persistedThreadGroupSet

	dir        string
	fileName   string
	threadGrps []*ThreadGroup

	mu sync.RWMutex
}

func NewThreadGroupSet(dirIn string, thrGroupNames []string) *ThreadGroupSet {
	set := &ThreadGroupSet{
		persisted:  persistedThreadGroupSet{ThreadNum: 0},
		dir:        dirIn,
		fileName:   threadGroupSetFileName,
		threadGrps: make([]*ThreadGroup, 0),
	}

	for _, thrGroupName := range thrGroupNames {
		grpDir := filepath.Join(dirIn, thrGroupName)
		thrGrp := newThreadGroup(set, thrGroupName, grpDir)
		set.threadGrps = append(set.threadGrps, thrGrp)
	}

	return set
}

// NewThread creates a new thread in the specified thread group
func (tgs *ThreadGroupSet) NewThread(thrGroupName string, thrName string) error {
	tgs.mu.Lock()
	defer tgs.mu.Unlock()

	for _, thrGroup := range tgs.threadGrps {
		if thrGroup.Name() == thrGroupName {
			return thrGroup.NewThread(thrName)
		}
	}

	return fmt.Errorf("No such thread group %v", thrGroupName)
}

func (tgs *ThreadGroupSet) MoveThread(thr Thread, srcThrGrpName, dstThrGrpName string) error {
	tgs.mu.Lock()
	defer tgs.mu.Unlock()

	var srcThrGrp, dstThrGrp *ThreadGroup
	for _, thrGroup := range tgs.threadGrps {
		if thrGroup.Name() == srcThrGrpName {
			srcThrGrp = thrGroup
		}
		if thrGroup.Name() == dstThrGrpName {
			dstThrGrp = thrGroup
		}
	}

	if srcThrGrp == nil {
		fmt.Errorf("No such thread group %v", srcThrGrpName)
	}
	if dstThrGrp == nil {
		fmt.Errorf("No such thread group %v", dstThrGrpName)
	}

	return srcThrGrp.MoveThread(thr, dstThrGrp)
}

// newThreadId generated a new, monotonically increasing, persistent thread id.
// callers should already hold a write lock on the thread group set's mutex
func (tgs *ThreadGroupSet) newThreadId() (string, error) {
	tgs.persisted.ThreadNum++
	err := tgs.save()
	if err != nil {
		return "", err
	}

	return strconv.FormatInt(tgs.persisted.ThreadNum, 10), nil
}

func (tgs *ThreadGroupSet) Threads(thrGroupNames []string) []Thread {
	tgs.mu.RLock()
	defer tgs.mu.RUnlock()

	ret := make([]Thread, 0)
	for _, thrGroup := range tgs.threadGrps {
		match := len(thrGroupNames) == 0
		for _, matchThrGroupName := range thrGroupNames {
			if thrGroup.Name() == matchThrGroupName {
				match = true
				break
			}
		}
		if !match {
			continue
		}

		ret = append(ret, thrGroup.Threads()...)
	}

	slices.SortFunc(ret, func(a, b Thread) int {
		return -a.AccessTime().Compare(b.AccessTime())
	})

	return ret
}

func (tgs *ThreadGroupSet) NonIdleThreadCount() int {
	tgs.mu.RLock()
	defer tgs.mu.RUnlock()

	count := 0
	for _, thrGroup := range tgs.threadGrps {
		count += thrGroup.NonIdleThreadCount()
	}

	return count
}

// Load restores persisted thread group set.
func (tgs *ThreadGroupSet) Load() error {
	filePath := filepath.Join(tgs.dir, tgs.fileName)

	tgs.mu.Lock()
	defer tgs.mu.Unlock()

	content, err := os.ReadFile(filePath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("failed to read thread group set (%v): %w", filePath, err)
		}
	} else {
		var persisted persistedThreadGroupSet
		if err := json.Unmarshal(content, &persisted); err != nil {
			return fmt.Errorf("failed to parse thread group set (%v): %w", filePath, err)
		}
		if persisted.ThreadNum <= 0 {
			persisted.ThreadNum = 1
		}

		tgs.persisted = persisted
	}

	return tgs.loadThreadGroups()
}

// save persists the thread group set fields to disk; callers should already
// hold a write lock on the thread group set's mutex.
func (tgs *ThreadGroupSet) save() error {
	content, err := json.Marshal(&tgs.persisted)
	if err != nil {
		return fmt.Errorf("failed to marshal thread group set: %w", err)
	}

	filePath := filepath.Join(tgs.dir, tgs.fileName)
	err = os.WriteFile(filePath, content, 0600)
	if err != nil {
		return fmt.Errorf("failed to save thread group set (%v): %w", filePath, err)
	}

	return nil
}

// loadThreadGroups loads threads for each thread group in the set. callers
// should already hold a write lock on the thread group set's mutex.
func (tgs *ThreadGroupSet) loadThreadGroups() error {
	for _, tg := range tgs.threadGrps {
		if err := tg.LoadThreads(); err != nil {
			return err
		}
	}

	return nil
}
