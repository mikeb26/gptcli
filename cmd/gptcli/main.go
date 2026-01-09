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
	CommandName       = "gptcli"
	KeyFileFmt        = ".%v.key"
	PrefsFile         = "prefs.json"
	ApprovePolicyFile = "approvals.json"
	ThreadsDir        = "threads"
	ArchiveDir        = "archive_threads"
	LogsDir           = "logs"
	AuditLogFile      = "audit.log"
)

type Prefs struct {
	SummarizePrior bool   `json:"summarize_prior"`
	Vendor         string `json:"vendor"`
	EnableAuditLog bool   `json:"enable_audit_log"`
}

type Toggles struct {
	summary    bool
	useColors  bool
	needConfig bool
}

type CliContext struct {
	ictx types.InternalContext

	ui      *ui.NcursesUI
	rootWin *gc.Window
	menu    *threadMenuUI

	prefs   Prefs
	toggles Toggles

	threadGroups       []*threads.ThreadGroup
	archiveThreadGroup *threads.ThreadGroup
	mainThreadGroup    *threads.ThreadGroup
	curThreadGroup     *threads.ThreadGroup

	asyncChatUIStates map[string]*asyncChatUIState
}

func NewCliContext(ctx context.Context) (*CliContext, error) {
	var err error
	var rootWinLocal *gc.Window
	rootWinLocal, err = gcInit()
	if err != nil {
		return nil, err
	}

	cliCtx := &CliContext{
		ui:      ui.NewNcursesUI(rootWinLocal),
		rootWin: rootWinLocal,
		toggles: Toggles{
			summary:    false,
			needConfig: true,
			useColors:  false,
		},
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
	cliCtx.menu = newThreadMenuUI(cliCtx)

	threadsDirLocal, err := getThreadsDir()
	if err != nil {
		threadsDirLocal = "/tmp"
	}
	archiveDirLocal, err := getArchiveDir()
	if err != nil {
		archiveDirLocal = "/tmp"
	}

	cliCtx.threadGroups = append(cliCtx.threadGroups,
		threads.NewThreadGroup("", threadsDirLocal))
	cliCtx.threadGroups = append(cliCtx.threadGroups,
		threads.NewThreadGroup("a", archiveDirLocal))

	cliCtx.mainThreadGroup = cliCtx.threadGroups[0]
	cliCtx.archiveThreadGroup = cliCtx.threadGroups[1]
	cliCtx.curThreadGroup = cliCtx.mainThreadGroup

	return cliCtx, nil
}

func (cliCtx *CliContext) load(ctx context.Context) error {

	cliCtx.toggles.needConfig = true
	err := cliCtx.loadPrefs()
	if err != nil {
		return err
	}
	keyText, err := loadKey(cliCtx.prefs.Vendor)
	if err != nil {
		return err
	}

	if cliCtx.prefs.EnableAuditLog {
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

	cliCtx.ictx.LlmPolicyStore = policyStore
	cliCtx.ictx.LlmVendor = cliCtx.prefs.Vendor
	cliCtx.ictx.LlmModel = internal.DefaultModels[cliCtx.prefs.Vendor]
	cliCtx.ictx.LlmApiKey = keyText
	cliCtx.ictx.LlmReasoningEffort = laclopenai.ReasoningEffortLevelMedium
	if cliCtx.prefs.EnableAuditLog {
		cliCtx.ictx.LlmAuditLogPath = auditLogPath
	}
	cliCtx.ictx.LlmBaseApprover = ui.NewUIApprover(cliCtx.ui)

	for _, thrGrp := range cliCtx.threadGroups {
		err := thrGrp.LoadThreads()
		if err != nil {
			return err
		}
	}
	cliCtx.toggles.needConfig = false

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
	cliCtx, err := NewCliContext(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v: Failed to initialize UI: %v\n", CommandName,
			err)
		os.Exit(1)
	}
	defer gcExit()

	err = cliCtx.load(ctx)
	if err != nil && !cliCtx.toggles.needConfig {
		fmt.Fprintf(os.Stderr, "%v: Failed to load: %v\n", CommandName, err)
		os.Exit(1)
	}

	showMenu(ctx, cliCtx, threadGroupString(cliCtx.curThreadGroup,
		false, false))
}
