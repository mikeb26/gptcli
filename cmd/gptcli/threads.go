/* Copyright © 2023-2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"

	"github.com/mikeb26/gptcli/internal/prompts"
	"github.com/mikeb26/gptcli/internal/types"
)

type GptCliThread struct {
	Name       string                 `json:"name"`
	CreateTime time.Time              `json:"ctime"`
	AccessTime time.Time              `json:"atime"`
	ModTime    time.Time              `json:"mtime"`
	Dialogue   []*types.GptCliMessage `json:"dialogue"`

	fileName string
}

type GptCliThreadGroup struct {
	prefix       string
	threads      []*GptCliThread
	totThreads   int
	dir          string
	curThreadNum int
}

func NewGptCliThreadGroup(prefixIn string, dirIn string) *GptCliThreadGroup {

	thrGrp := &GptCliThreadGroup{
		prefix:       prefixIn,
		threads:      make([]*GptCliThread, 0),
		totThreads:   0,
		dir:          dirIn,
		curThreadNum: 0,
	}

	return thrGrp
}

func (thrGrp *GptCliThreadGroup) loadThreads() error {
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

func (thread *GptCliThread) save(dir string) error {
	threadFileContent, err := json.Marshal(thread)
	if err != nil {
		return fmt.Errorf("Failed to save thread %v: %w", thread.Name, err)
	}

	filePath := filepath.Join(dir, thread.fileName)
	err = os.WriteFile(filePath, threadFileContent, 0600)
	if err != nil {
		return fmt.Errorf("Failed to save thread %v(%v): %w", thread.Name,
			filePath, err)
	}

	return nil
}

func (thread *GptCliThread) remove(dir string) error {
	filePath := filepath.Join(dir, thread.fileName)
	err := os.Remove(filePath)
	if err != nil {
		return fmt.Errorf("Failed to delete thread %v(%v): %w", thread.Name,
			filePath, err)
	}

	return nil
}

func lsThreadsMain(ctx context.Context, gptCliCtx *GptCliContext,
	args []string) error {

	if gptCliCtx.mainThreadGroup.totThreads == 0 {
		fmt.Printf("%v.\n", ErrNoThreadsExist)
		return nil
	}

	showAll := false

	f := flag.NewFlagSet("ls", flag.ContinueOnError)
	f.BoolVar(&showAll, "all", false, "Also show archive threads")
	err := f.Parse(args[1:])
	if err != nil {
		return err
	}

	fmt.Printf("%v", gptCliCtx.mainThreadGroup.String(true, !showAll))
	if showAll {
		fmt.Printf("%v", gptCliCtx.archiveThreadGroup.String(false, true))
	}

	return nil
}

func threadGroupHeaderString() string {
	var sb strings.Builder

	sb.WriteString(RowSpacer)
	sb.WriteString(fmt.Sprintf(RowFmt, "Thread#", "Last Accessed", "Last Modified",
		"Created", "Name"))

	sb.WriteString(RowSpacer)

	return sb.String()
}

func threadGroupFooterString() string {
	return RowSpacer
}

func (t *GptCliThread) HeaderString(threadNum string) string {
	cTime := t.CreateTime.Format("01/02/2006 03:04pm")
	aTime := t.AccessTime.Format("01/02/2006 03:04pm")
	mTime := t.ModTime.Format("01/02/2006 03:04pm")
	today := time.Now().UTC().Truncate(24 * time.Hour).Format("01/02/2006")
	yesterday := time.Now().UTC().Add(-24 * time.Hour).Truncate(24 * time.Hour).Format("01/02/2006")
	cTime = strings.ReplaceAll(cTime, today, "Today")
	aTime = strings.ReplaceAll(aTime, today, "Today")
	mTime = strings.ReplaceAll(mTime, today, "Today")
	cTime = strings.ReplaceAll(cTime, yesterday, "Yesterday")
	aTime = strings.ReplaceAll(aTime, yesterday, "Yesterday")
	mTime = strings.ReplaceAll(mTime, yesterday, "Yesterday")

	return fmt.Sprintf(RowFmt, threadNum, aTime, mTime, cTime, t.Name)
}

func (thrGrp *GptCliThreadGroup) String(header bool, footer bool) string {
	var sb strings.Builder

	if header {
		sb.WriteString(threadGroupHeaderString())
	}

	for idx, t := range thrGrp.threads {
		threadNum := fmt.Sprintf("%v%v", thrGrp.prefix, idx+1)
		sb.WriteString(t.HeaderString(threadNum))
	}

	if footer {
		sb.WriteString(threadGroupFooterString())
	}

	return sb.String()
}

func parseThreadNum(gptCliCtx *GptCliContext,
	userInput string) (*GptCliThreadGroup, int, error) {

	prefix := strings.TrimRight(userInput, "0123456789")
	suffix := userInput[len(prefix):]
	threadNum, err := strconv.ParseUint(suffix, 10, 64)
	if err != nil {
		return nil, 0, fmt.Errorf(ThreadParseErrFmt, userInput)
	}

	for _, thrGrp := range gptCliCtx.threadGroups {
		if prefix == thrGrp.prefix {
			return thrGrp, int(threadNum), nil
		}
	}

	return nil, 0, fmt.Errorf(ThreadParseErrFmt, userInput)
}

func threadSwitchMain(ctx context.Context, gptCliCtx *GptCliContext,
	args []string) error {

	if len(args) != 2 {
		return fmt.Errorf("Syntax is 'thread <thread#>' e.g. 'thread 1'\n")
	}
	thrGrp, threadNum, err := parseThreadNum(gptCliCtx, args[1])
	if err != nil {
		return err
	}
	if gptCliCtx.curThreadGroup != thrGrp {
		gptCliCtx.curThreadGroup = thrGrp
	}
	return thrGrp.threadSwitch(int(threadNum))
}

func (thrGrp *GptCliThreadGroup) threadSwitch(threadNum int) error {
	if threadNum > thrGrp.totThreads || threadNum == 0 {
		threadNumPrint := fmt.Sprintf("%v%v", thrGrp.prefix, threadNum)
		return fmt.Errorf(ThreadNoExistErrFmt, threadNumPrint)
	}

	thrGrp.curThreadNum = threadNum
	thread := thrGrp.threads[thrGrp.curThreadNum-1]
	thread.AccessTime = time.Now()
	err := thread.save(thrGrp.dir)
	if err != nil {
		return err
	}

	printToScreen(thread.String())

	return nil
}

func (thread *GptCliThread) String() string {
	var sb strings.Builder

	for _, msg := range thread.Dialogue {
		if msg.Role == types.GptCliMessageRoleSystem {
			continue
		}

		if msg.Role == types.GptCliMessageRoleAssistant {
			blocks := splitBlocks(msg.Content)
			for idx, b := range blocks {
				if idx%2 == 0 {
					sb.WriteString(color.BlueString("%v\n", b))
				} else {
					sb.WriteString(color.GreenString("%v\n", b))
				}
			}
			continue
		} else if msg.Role == types.GptCliMessageRoleUser {
			sb.WriteString(fmt.Sprintf("gptcli/%v> %v\n",
				thread.Name, msg.Content))
		}
	}

	return sb.String()
}

func newThreadMain(ctx context.Context, gptCliCtx *GptCliContext,
	args []string) error {

	if gptCliCtx.needConfig {
		return fmt.Errorf("You must run 'config' before creating a thread.\n")
	}

	fmt.Printf("Enter new thread's name: ")
	name, err := gptCliCtx.input.ReadString('\n')
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
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
	gptCliCtx.mainThreadGroup.curThreadNum =
		gptCliCtx.mainThreadGroup.addThread(curThread)

	return nil
}

func (thrGrp *GptCliThreadGroup) addThread(curThread *GptCliThread) int {
	thrGrp.totThreads++
	thrGrp.threads = append(thrGrp.threads, curThread)

	return thrGrp.totThreads
}

func archiveThreadMain(ctx context.Context, gptCliCtx *GptCliContext,
	args []string) error {

	if len(args) != 2 {
		return fmt.Errorf("Syntax is 'archive <thread#>' e.g. 'archive 1'\n")
	}
	thrGrp, threadNum, err := parseThreadNum(gptCliCtx, args[1])
	if err != nil {
		return err
	}

	if thrGrp == gptCliCtx.archiveThreadGroup {
		return fmt.Errorf("gptcli: Thread already archived")
	} else if thrGrp != gptCliCtx.mainThreadGroup {
		panic("BUG: archiveThreadMain() only supports 2 thread groups currently")
	}

	err = thrGrp.moveThread(int(threadNum), gptCliCtx.archiveThreadGroup)
	if err != nil {
		return fmt.Errorf("gptcli: Failed to archive thread: %w", err)
	}

	fmt.Printf("gptcli: Archived thread %v. Remaining threads renumbered.\n",
		threadNum)

	lsArgs := []string{"ls"}
	return lsThreadsMain(ctx, gptCliCtx, lsArgs)
}

func unarchiveThreadMain(ctx context.Context, gptCliCtx *GptCliContext,
	args []string) error {

	if len(args) != 2 {
		return fmt.Errorf("Syntax is 'unarchive a<thread#>' e.g. 'unarchive 1'\n")
	}
	thrGrp, threadNum, err := parseThreadNum(gptCliCtx, args[1])
	if err != nil {
		return err
	}

	if thrGrp == gptCliCtx.mainThreadGroup {
		return fmt.Errorf("gptcli: Thread already unarchived")
	} else if thrGrp != gptCliCtx.archiveThreadGroup {
		panic("BUG: unarchiveThreadMain() only supports 2 thread groups currently")
	}

	err = thrGrp.moveThread(int(threadNum), gptCliCtx.mainThreadGroup)
	if err != nil {
		return fmt.Errorf("gptcli: Failed to unarchive thread: %w", err)
	}

	fmt.Printf("gptcli: Unarchived thread %v. Remaining threads renumbered.\n",
		threadNum)

	lsArgs := []string{"ls"}
	return lsThreadsMain(ctx, gptCliCtx, lsArgs)
}

func (srcThrGrp *GptCliThreadGroup) moveThread(threadNum int,
	dstThrGrp *GptCliThreadGroup) error {

	if threadNum > srcThrGrp.totThreads || threadNum == 0 {
		threadNumPrint := fmt.Sprintf("%v%v", srcThrGrp.prefix, threadNum)
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

	return srcThrGrp.loadThreads()
}

func interactiveThreadWork(ctx context.Context,
	gptCliCtx *GptCliContext, prompt string) error {

	reqMsg := &types.GptCliMessage{
		Role:    types.GptCliMessageRoleUser,
		Content: prompt,
	}

	thrGrp := gptCliCtx.curThreadGroup
	if thrGrp == gptCliCtx.archiveThreadGroup {
		return fmt.Errorf("Cannot edit archived thread; use unarchive first")
	}
	thread := thrGrp.threads[thrGrp.curThreadNum-1]
	fullDialogue := thread.Dialogue
	summaryDialogue := fullDialogue

	fullDialogue = append(fullDialogue, reqMsg)
	workingDialogue := fullDialogue

	var err error
	if gptCliCtx.curSummaryToggle && len(fullDialogue) > 2 {
		summaryDialogue, err = summarizeDialogue(ctx, gptCliCtx, summaryDialogue)
		if err != nil {
			return err
		}
		summaryDialogue = append(summaryDialogue, reqMsg)
		workingDialogue = summaryDialogue
	}

	var replyMsg *types.GptCliMessage
	fmt.Printf("gptcli: processing...\n")
	// @todo need ReasoningEffort: "high",
	replyMsg, err = gptCliCtx.client.CreateChatCompletion(ctx, workingDialogue)
	if err != nil {
		return err
	}

	fullDialogue = append(fullDialogue, replyMsg)

	var sb strings.Builder
	blocks := splitBlocks(replyMsg.Content)
	for idx, b := range blocks {
		if idx%2 == 0 {
			sb.WriteString(color.BlueString("%v\n", b))
		} else {
			sb.WriteString(color.GreenString("%v\n", b))
		}
	}

	printToScreen(sb.String())

	thread.Dialogue = fullDialogue
	thread.ModTime = time.Now()
	thread.AccessTime = time.Now()

	err = thread.save(thrGrp.dir)
	if err != nil {
		return err
	}

	return nil
}

func catMain(ctx context.Context, gptCliCtx *GptCliContext,
	args []string) error {

	var thrGrp *GptCliThreadGroup
	var threadNum int
	var err error

	if len(args) > 2 {
		return fmt.Errorf("Syntax is 'cat <thread#>' e.g. 'cat 1'\n")
	} else if len(args) == 2 {
		thrGrp, threadNum, err = parseThreadNum(gptCliCtx, args[1])
		if err != nil {
			return err
		}
	} else {
		thrGrp = gptCliCtx.curThreadGroup
		threadNum = thrGrp.curThreadNum
	}

	if threadNum > thrGrp.totThreads {
		threadNumPrint := fmt.Sprintf("%v%v", thrGrp.prefix, threadNum)
		return fmt.Errorf(ThreadNoExistErrFmt, threadNumPrint)
	} else if threadNum == 0 {
		return fmt.Errorf("No thread is currently selected. Select one with 'thread <thread#>'.")
	}

	thread := thrGrp.threads[threadNum-1]
	thread.AccessTime = time.Now()
	err = thread.save(thrGrp.dir)
	if err != nil {
		return err
	}

	printToScreen(thread.String())

	return nil
}
