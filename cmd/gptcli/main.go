/* Copyright © 2023-2025 Mike Brown. All Rights Reserved.
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
	"github.com/mikeb26/gptcli/internal/llmclient"
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

type GptCliContext struct {
	client types.GptCliAIClient
	// For ncurses, the underlying approver must only be invoked
	// from the ncurses goroutine; AsyncApprover forwards approval requests
	// over a channel so the ncurses goroutine can serve them.
	asyncApprover *am.AsyncApprover

	// realUI is the concrete ncurses UI implementation owned by the
	// ncurses/rendering goroutine.
	realUI *ui.NcursesUI

	scr                *gc.Window
	needConfig         bool
	curSummaryToggle   bool
	prefs              Prefs
	threadGroups       []*threads.GptCliThreadGroup
	archiveThreadGroup *threads.GptCliThreadGroup
	mainThreadGroup    *threads.GptCliThreadGroup
	curThreadGroup     *threads.GptCliThreadGroup
}

func NewGptCliContext(ctx context.Context) *GptCliContext {

	//	inputLocal := bufio.NewReader(os.Stdin)

	var err error
	var scrLocal *gc.Window
	scrLocal, err = gcInit()
	if err != nil {
		panic("fix me")
	}
	// real ncurses UI (must only be used from the ncurses goroutine)
	realUILocal := ui.NewNcursesUI(scrLocal)

	gptCliCtx := &GptCliContext{
		client: nil,
		//		input:            inputLocal,
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
		threadGroups:       make([]*threads.GptCliThreadGroup, 0),
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
		threads.NewGptCliThreadGroup("", threadsDirLocal))
	gptCliCtx.threadGroups = append(gptCliCtx.threadGroups,
		threads.NewGptCliThreadGroup("a", archiveDirLocal))

	gptCliCtx.mainThreadGroup = gptCliCtx.threadGroups[0]
	gptCliCtx.archiveThreadGroup = gptCliCtx.threadGroups[1]
	gptCliCtx.curThreadGroup = gptCliCtx.mainThreadGroup
	err = gptCliCtx.loadPrefs()
	if err == nil {
		gptCliCtx.needConfig = false
	}

	return gptCliCtx
}

func (gptCliCtx *GptCliContext) load(ctx context.Context) error {

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
			return fmt.Errorf("Could not create logs directory %v: %w", auditLogsDir, err)
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
	var approver am.Approver
	approver = ui.NewUIApprover(gptCliCtx.realUI)
	approver = am.NewPolicyStoreApprover(approver, policyStore)
	gptCliCtx.asyncApprover = am.NewAsyncApprover(approver)

	gptCliCtx.client = llmclient.NewEINOClient(ctx, gptCliCtx.prefs.Vendor,
		gptCliCtx.asyncApprover, keyText,
		internal.DefaultModels[gptCliCtx.prefs.Vendor], 0,
		gptCliCtx.prefs.EnableAuditLog, auditLogPath)

	for _, thrGrp := range gptCliCtx.threadGroups {
		err := thrGrp.LoadThreads()
		if err != nil {
			return err
		}
	}
	gptCliCtx.needConfig = false

	return nil
}

func summaryToggleMain(ctx context.Context, gptCliCtx *GptCliContext,
	args []string) error {

	// @todo convert to dialogue
	usageErr := fmt.Errorf("Syntax is 'summary [<on|off>]' e.g. 'summary on'\n")

	if len(args) == 1 {
		gptCliCtx.curSummaryToggle = !gptCliCtx.curSummaryToggle
	} else if len(args) != 2 {
		return usageErr
	} else {
		if strings.ToLower(args[1]) == "on" {
			gptCliCtx.curSummaryToggle = true
		} else if strings.ToLower(args[1]) == "off" {
			gptCliCtx.curSummaryToggle = false
		} else {
			return usageErr
		}
	}

	if gptCliCtx.curSummaryToggle {
		fmt.Printf("summaries enabled; summaries of the thread history are sent for followups in order to reduce costs.\n")
	} else {
		fmt.Printf("summaries disabled; the full thread history is sent for	followups in order to get more precise responses.\n")
	}

	return nil
}

func threadContainsSearchStr(t *threads.GptCliThread, searchStr string) bool {
	for _, msg := range t.Dialogue {
		if msg.Role == types.GptCliMessageRoleSystem {
			continue
		}

		if strings.Contains(msg.Content, searchStr) {
			return true
		}
	}

	return false
}

func searchMain(ctx context.Context, gptCliCtx *GptCliContext,
	args []string) error {

	// @todo convert to dialogue. also need search results submenu
	usageErr := fmt.Errorf("Syntax is 'search <search_string>[,<search_string>...] e.g. 'search foo'\n")

	if len(args) < 2 {
		return usageErr
	}
	searchStrs := args[1:]

	var sb strings.Builder

	sb.WriteString(threads.ThreadGroupHeaderString(true))

	for _, thrGrp := range gptCliCtx.threadGroups {
		for tidx, t := range thrGrp.Threads() {
			count := 0
			for _, searchStr := range searchStrs {
				if threadContainsSearchStr(t, searchStr) {
					count++
				}
			}
			if count == len(searchStrs) {
				threadNum := fmt.Sprintf("%v%v", thrGrp.Prefix, tidx+1)
				sb.WriteString(t.HeaderString(threadNum))
			}
		}
	}

	sb.WriteString(threads.ThreadGroupFooterString())

	fmt.Printf("%v", sb.String())

	return nil
}

func reasoningMain(ctx context.Context, gptCliCtx *GptCliContext,
	args []string) error {

	// @todo convert to options dialogue
	usageErr := fmt.Errorf("Syntax is 'reasoning <low|medium|high>'\n")

	if len(args) != 2 {
		return usageErr
	}
	reasoningLvl := laclopenai.ReasoningEffortLevel(strings.ToLower(args[1]))
	switch reasoningLvl {
	case laclopenai.ReasoningEffortLevelHigh:
		fallthrough
	case laclopenai.ReasoningEffortLevelMedium:
		fallthrough
	case laclopenai.ReasoningEffortLevelLow:
		break
	default:
		return fmt.Errorf("Unknown reasoning effort: %v", args[1])
	}

	gptCliCtx.client.SetReasoning(reasoningLvl)
	return nil
}

func main() {
	ctx := context.Background()
	gptCliCtx := NewGptCliContext(ctx)
	defer gcExit()

	// @todo needConfig?
	err := gptCliCtx.load(ctx)
	if err != nil && !gptCliCtx.needConfig {
		fmt.Fprintf(os.Stderr, "gptcli: Failed to load: %v\n", err)
		os.Exit(1)
	}

	menuMain(ctx, gptCliCtx, make([]string, 2))
}
