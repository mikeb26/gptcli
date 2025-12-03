/* Copyright © 2023-2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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

func newThreadMain(ctx context.Context, gptCliCtx *GptCliContext,
	args []string) error {

	if gptCliCtx.needConfig {
		return fmt.Errorf("You must run 'config' before creating a thread.\n")
	}

	name, err := gptCliCtx.ui.Get("Enter new thread's name: ")
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

// @todo need ux
//  unarchiveThreadMain()

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
