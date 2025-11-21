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

// RenderBlockKind identifies the semantic type of a block of text in a
// thread dialogue. This is UI-agnostic so that different frontends
// (classic CLI, ncurses, etc.) can render the same logical content with
// their own styling.
type RenderBlockKind int

const (
	RenderBlockUserPrompt RenderBlockKind = iota
	RenderBlockAssistantText
	RenderBlockAssistantCode
)

// RenderBlock represents a contiguous span of text with a particular
// semantic role. It does not contain any ANSI color or formatting
// information; callers are expected to style it appropriately.
type RenderBlock struct {
	Kind RenderBlockKind
	Text string
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

func threadGroupHeaderString(includeSpacers bool) string {
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

func threadGroupFooterString() string {
	return RowSpacer
}

func (t *GptCliThread) HeaderString(threadNum string) string {
	now := time.Now()

	cTime := formatHeaderTime(t.CreateTime, now)
	aTime := formatHeaderTime(t.AccessTime, now)
	mTime := formatHeaderTime(t.ModTime, now)

	return fmt.Sprintf(RowFmt, threadNum, aTime, mTime, cTime, t.Name)
}

// formatHeaderTime renders a timestamp for use in the thread list header.
// If the time falls on the same local calendar day as "now", the date
// portion is replaced with "Today". If it falls on the preceding
// calendar day, it is replaced with "Yesterday". Otherwise, the full
// date is shown. Calendar-day comparisons are done in the local time
// zone associated with "now" to avoid off-by-one errors around
// midnight or when using non-UTC locations.
func formatHeaderTime(ts time.Time, now time.Time) string {
	// Normalize the target time into the same location as "now" so
	// that calendar-day comparisons are meaningful.
	ts = ts.In(now.Location())

	full := ts.Format("01/02/2006 03:04pm")
	datePart := ts.Format("01/02/2006")

	y, m, d := now.Date()
	todayY, todayM, todayD := y, m, d
	yest := now.AddDate(0, 0, -1)
	yestY, yestM, yestD := yest.Date()
	ty, tm, td := ts.Date()

	switch {
	case ty == todayY && tm == todayM && td == todayD:
		return strings.Replace(full, datePart, "Today", 1)
	case ty == yestY && tm == yestM && td == yestD:
		return strings.Replace(full, datePart, "Yesterday", 1)
	default:
		return full
	}
}

func (thrGrp *GptCliThreadGroup) String(header bool, footer bool) string {
	var sb strings.Builder

	if header {
		sb.WriteString(threadGroupHeaderString(true))
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

// activateThread updates the thread group's current thread state,
// refreshes the access time, and persists the thread to disk. It
// performs no user-facing I/O and is therefore safe to call from
// different UIs (CLI, ncurses, etc.).
func (thrGrp *GptCliThreadGroup) activateThread(threadNum int) (*GptCliThread, error) {
	if threadNum > thrGrp.totThreads || threadNum == 0 {
		threadNumPrint := fmt.Sprintf("%v%v", thrGrp.prefix, threadNum)
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
	thread, err := thrGrp.activateThread(threadNum)
	if err != nil {
		return err
	}

	printToScreen(thread.String())

	return nil
}

func (thread *GptCliThread) String() string {
	var sb strings.Builder

	blocks := thread.RenderBlocks()
	for _, b := range blocks {
		switch b.Kind {
		case RenderBlockUserPrompt:
			sb.WriteString(fmt.Sprintf("gptcli/%v> %v\n", thread.Name, b.Text))
		case RenderBlockAssistantText:
			sb.WriteString(color.BlueString("%v\n", b.Text))
		case RenderBlockAssistantCode:
			sb.WriteString(color.GreenString("%v\n", b.Text))
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

	return createNewThread(gptCliCtx, name)
}

// createNewThread encapsulates the logic to allocate and register a new
// thread in the main thread group. It is used both by the CLI "new"
// subcommand and the ncurses menu UI so their behavior stays in sync.
func createNewThread(gptCliCtx *GptCliContext, name string) error {
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

// RenderBlocks flattens the thread dialogue into a sequence of
// RenderBlocks that capture the semantic structure (user prompt,
// assistant text, assistant code) without imposing any particular UI
// representation.
func (thread *GptCliThread) RenderBlocks() []RenderBlock {
	blocks := make([]RenderBlock, 0)

	for _, msg := range thread.Dialogue {
		if msg.Role == types.GptCliMessageRoleSystem {
			continue
		}

		switch msg.Role {
		case types.GptCliMessageRoleUser:
			blocks = append(blocks, RenderBlock{
				Kind: RenderBlockUserPrompt,
				Text: msg.Content,
			})
		case types.GptCliMessageRoleAssistant:
			parts := splitBlocks(msg.Content)
			for idx, p := range parts {
				kind := RenderBlockAssistantText
				if idx%2 == 1 {
					kind = RenderBlockAssistantCode
				}
				blocks = append(blocks, RenderBlock{
					Kind: kind,
					Text: p,
				})
			}
		}
	}

	return blocks
}

// ChatOnceInCurrentThread encapsulates the core request/response flow
// for sending a prompt to the current thread, updating dialogue
// history, and persisting the result. It performs no direct terminal
// I/O so callers can render the assistant reply however they choose.
func (gptCliCtx *GptCliContext) ChatOnceInCurrentThread(
	ctx context.Context, prompt string,
) (*types.GptCliMessage, error) {

	thrGrp := gptCliCtx.curThreadGroup
	if thrGrp == gptCliCtx.archiveThreadGroup {
		return nil, fmt.Errorf("Cannot edit archived thread; use unarchive first")
	}
	if thrGrp.curThreadNum == 0 || thrGrp.curThreadNum > thrGrp.totThreads {
		return nil, fmt.Errorf("No thread is currently selected. Select one with 'thread <thread#>'.")
	}

	reqMsg := &types.GptCliMessage{
		Role:    types.GptCliMessageRoleUser,
		Content: prompt,
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
			return nil, err
		}
		summaryDialogue = append(summaryDialogue, reqMsg)
		workingDialogue = summaryDialogue
	}

	replyMsg, err := gptCliCtx.client.CreateChatCompletion(ctx, workingDialogue)
	if err != nil {
		return nil, err
	}

	fullDialogue = append(fullDialogue, replyMsg)
	thread.Dialogue = fullDialogue
	thread.ModTime = time.Now()
	thread.AccessTime = time.Now()

	if err := thread.save(thrGrp.dir); err != nil {
		return nil, err
	}

	return replyMsg, nil
}

func interactiveThreadWork(ctx context.Context,
	gptCliCtx *GptCliContext, prompt string) error {
	fmt.Printf("gptcli: processing...\n")

	replyMsg, err := gptCliCtx.ChatOnceInCurrentThread(ctx, prompt)
	if err != nil {
		return err
	}

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
