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
	"strings"
	"time"

	"github.com/mikeb26/gptcli/internal/prompts"
	"github.com/mikeb26/gptcli/internal/types"
)

type GptCliThreadGroup struct {
	Prefix       string
	threads      []*GptCliThread
	totThreads   int
	dir          string
	curThreadNum int
}

func NewGptCliThreadGroup(PrefixIn string, dirIn string) *GptCliThreadGroup {

	thrGrp := &GptCliThreadGroup{
		Prefix:       PrefixIn,
		threads:      make([]*GptCliThread, 0),
		totThreads:   0,
		dir:          dirIn,
		curThreadNum: 0,
	}

	return thrGrp
}

func (thrGrp *GptCliThreadGroup) Threads() []*GptCliThread {
	return thrGrp.threads
}

func (thrGrp *GptCliThreadGroup) LoadThreads() error {
	thrGrp.curThreadNum = 0
	thrGrp.totThreads = 0
	thrGrp.threads = make([]*GptCliThread, 0)

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

		var thread GptCliThread
		err = json.Unmarshal(threadFileText, &thread)
		if err != nil {
			return fmt.Errorf("Failed to parse %v: %w", fullpath, err)
		}
		thread.fileName = genUniqFileName(thread.Name, thread.CreateTime)
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

func ThreadGroupHeaderString(includeSpacers bool) string {
	var sb strings.Builder

	if includeSpacers {
		sb.WriteString(RowSpacer)
	}
	sb.WriteString(fmt.Sprintf(RowFmt, "Thread#", "Last Accessed", "Last Modified",
		"Created", "Name"))

	if includeSpacers {
		sb.WriteString(RowSpacer)
	}

	return sb.String()
}

func ThreadGroupFooterString() string {
	return RowSpacer
}

func (thrGrp *GptCliThreadGroup) String(header bool, footer bool) string {
	var sb strings.Builder

	if header {
		sb.WriteString(ThreadGroupHeaderString(true))
	}

	for idx, t := range thrGrp.threads {
		threadNum := fmt.Sprintf("%v%v", thrGrp.Prefix, idx+1)
		sb.WriteString(t.HeaderString(threadNum))
	}

	if footer {
		sb.WriteString(ThreadGroupFooterString())
	}

	return sb.String()
}

// activateThread updates the thread group's current thread state,
// refreshes the access time, and persists the thread to disk. It
// performs no user-facing I/O and is therefore safe to call from
// different UIs (CLI, ncurses, etc.).
func (thrGrp *GptCliThreadGroup) ActivateThread(threadNum int) (*GptCliThread, error) {
	if threadNum > thrGrp.totThreads || threadNum == 0 {
		threadNumPrint := fmt.Sprintf("%v%v", thrGrp.Prefix, threadNum)
		return nil, fmt.Errorf(ThreadNoExistErrFmt, threadNumPrint)
	}

	thrGrp.curThreadNum = threadNum
	thread := thrGrp.threads[thrGrp.curThreadNum-1]
	thread.AccessTime = time.Now()
	if err := thread.save(thrGrp.dir); err != nil {
		return nil, err
	}

	return thread, nil
}

// NewThread encapsulates the logic to allocate and register a new
// thread in the main thread group. It is used both by the CLI "new"
// subcommand and the ncurses menu UI so their behavior stays in sync.
func (thrGrp *GptCliThreadGroup) NewThread(name string) error {
	cTime := time.Now()
	fileName := genUniqFileName(name, cTime)

	dialogue := []*types.GptCliMessage{
		{Role: types.GptCliMessageRoleSystem,
			Content: prompts.SystemMsg},
	}

	curThread := &GptCliThread{
		Name:       name,
		CreateTime: cTime,
		AccessTime: cTime,
		ModTime:    cTime,
		Dialogue:   dialogue,
		fileName:   fileName,
	}

	thrGrp.curThreadNum = thrGrp.addThread(curThread)

	return nil
}

func (thrGrp *GptCliThreadGroup) addThread(curThread *GptCliThread) int {
	thrGrp.totThreads++
	thrGrp.threads = append(thrGrp.threads, curThread)

	return thrGrp.totThreads
}

// @todo need ux
//  unarchiveThreadMain()

func (thrGrp *GptCliThreadGroup) Count() int {
	return thrGrp.totThreads
}

func (srcThrGrp *GptCliThreadGroup) MoveThread(threadNum int,
	dstThrGrp *GptCliThreadGroup) error {

	if threadNum > srcThrGrp.totThreads || threadNum == 0 {
		threadNumPrint := fmt.Sprintf("%v%v", srcThrGrp.Prefix, threadNum)
		return fmt.Errorf(ThreadNoExistErrFmt, threadNumPrint)
	}

	thread := srcThrGrp.threads[threadNum-1]

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

	return srcThrGrp.LoadThreads()
}
