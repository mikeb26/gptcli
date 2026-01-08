/* Copyright © 2023-2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"

	laclopenai "github.com/cloudwego/eino-ext/libs/acl/openai"
	gc "github.com/gbin/goncurses"

	"github.com/mikeb26/gptcli/internal"
	"github.com/mikeb26/gptcli/internal/am"
	"github.com/mikeb26/gptcli/internal/threads"
	"github.com/mikeb26/gptcli/internal/types"
	"github.com/mikeb26/gptcli/internal/ui"
)

const (
	CommandName           = "gptcli"
	KeyFileFmt            = ".%v.key"
	PrefsFile             = "prefs.json"
	ApprovePolicyFile     = "approvals.json"
	ThreadsDir            = "threads"
	ArchiveDir            = "archive_threads"
	LogsDir               = "logs"
	AuditLogFile          = "audit.log"
	CodeBlockDelim        = "```"
	CodeBlockDelimNewline = "```\n"
	ThreadParseErrFmt     = "Could not parse %v. Please enter a valid thread number.\n"
	ThreadNoExistErrFmt   = "Thread %v does not exist. To list threads try 'ls'.\n"
)

type Prefs struct {
	SummarizePrior bool   `json:"summarize_prior"`
	Vendor         string `json:"vendor"`
	EnableAuditLog bool   `json:"enable_audit_log"`
}

type CliContext struct {
	ictx types.InternalContext

	// realUI is the concrete ncurses UI implementation owned by the
	// ncurses/rendering goroutine.
	realUI *ui.NcursesUI

	scr                *gc.Window
	needConfig         bool
	curSummaryToggle   bool
	prefs              Prefs
	threadGroups       []*threads.ThreadGroup
	archiveThreadGroup *threads.ThreadGroup
	mainThreadGroup    *threads.ThreadGroup
	curThreadGroup     *threads.ThreadGroup

	// asyncChatUIStates tracks per-thread ncurses UI state for in-flight
	// async chats.
	asyncChatUIStates map[string]*asyncChatUIState
}

func NewCliContext(ctx context.Context) (*CliContext, error) {

	//	inputLocal := bufio.NewReader(os.Stdin)

	var err error
	var scrLocal *gc.Window
	scrLocal, err = gcInit()
	if err != nil {
		return nil, err
	}
	// real ncurses UI (must only be used from the ncurses goroutine)
	realUILocal := ui.NewNcursesUI(scrLocal)

	gptCliCtx := &CliContext{
		realUI:           realUILocal,
		scr:              scrLocal,
		needConfig:       true,
		curSummaryToggle: false,
		prefs: Prefs{
			SummarizePrior: false,
			Vendor:         internal.DefaultVendor,
			EnableAuditLog: true,
		},
		archiveThreadGroup: nil,
		mainThreadGroup:    nil,
		curThreadGroup:     nil,
		threadGroups:       make([]*threads.ThreadGroup, 0),
		asyncChatUIStates:  make(map[string]*asyncChatUIState),
	}

	threadsDirLocal, err := getThreadsDir()
	if err != nil {
		threadsDirLocal = "/tmp"
	}
	archiveDirLocal, err := getArchiveDir()
	if err != nil {
		archiveDirLocal = "/tmp"
	}

	gptCliCtx.threadGroups = append(gptCliCtx.threadGroups,
		threads.NewThreadGroup("", threadsDirLocal))
	gptCliCtx.threadGroups = append(gptCliCtx.threadGroups,
		threads.NewThreadGroup("a", archiveDirLocal))

	gptCliCtx.mainThreadGroup = gptCliCtx.threadGroups[0]
	gptCliCtx.archiveThreadGroup = gptCliCtx.threadGroups[1]
	gptCliCtx.curThreadGroup = gptCliCtx.mainThreadGroup
	err = gptCliCtx.loadPrefs()
	if err == nil {
		gptCliCtx.needConfig = false
	}

	return gptCliCtx, nil
}

func (gptCliCtx *CliContext) load(ctx context.Context) error {

	gptCliCtx.needConfig = true
	err := gptCliCtx.loadPrefs()
	if err != nil {
		return err
	}
	keyText, err := loadKey(gptCliCtx.prefs.Vendor)
	if err != nil {
		return err
	}

	if gptCliCtx.prefs.EnableAuditLog {
		auditLogsDir, err := getLogsDir()
		if err != nil {
			return err
		}
		err = os.MkdirAll(auditLogsDir, 0700)
		if err != nil {
			return fmt.Errorf("%w %v: %w", ErrCouldNotCreateLogsDir, auditLogsDir, err)
		}
	}

	auditLogPath, err := getAuditLogPath()
	if err != nil {
		return err
	}

	policyPath, err := getApprovePolicyPath()
	if err != nil {
		return err
	}
	policyStore, err := am.NewJSONApprovalPolicyStore(policyPath)
	if err != nil {
		return err
	}

	gptCliCtx.ictx.LlmPolicyStore = policyStore
	gptCliCtx.ictx.LlmVendor = gptCliCtx.prefs.Vendor
	gptCliCtx.ictx.LlmModel = internal.DefaultModels[gptCliCtx.prefs.Vendor]
	gptCliCtx.ictx.LlmApiKey = keyText
	gptCliCtx.ictx.LlmReasoningEffort = laclopenai.ReasoningEffortLevelMedium
	if gptCliCtx.prefs.EnableAuditLog {
		gptCliCtx.ictx.LlmAuditLogPath = auditLogPath
	}
	gptCliCtx.ictx.LlmBaseApprover = ui.NewUIApprover(gptCliCtx.realUI)

	for _, thrGrp := range gptCliCtx.threadGroups {
		err := thrGrp.LoadThreads()
		if err != nil {
			return err
		}
	}
	gptCliCtx.needConfig = false

	return nil
}

func threadContainsSearchStr(t threads.Thread, searchStr string) bool {
	for _, msg := range t.Dialogue() {
		if msg.Role == types.LlmRoleSystem {
			continue
		}

		if strings.Contains(msg.Content, searchStr) {
			return true
		}
	}

	return false
}

func main() {
	ctx := context.Background()
	gptCliCtx, err := NewCliContext(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gptcli: Failed to initialize UI: %v\n", err)
		os.Exit(1)
	}
	defer gcExit()

	// @todo needConfig?
	err = gptCliCtx.load(ctx)
	if err != nil && !gptCliCtx.needConfig {
		fmt.Fprintf(os.Stderr, "gptcli: Failed to load: %v\n", err)
		os.Exit(1)
	}

	menuMain(ctx, gptCliCtx, make([]string, 2))
}
