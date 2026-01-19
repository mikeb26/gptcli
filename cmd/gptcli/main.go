/* Copyright © 2023-2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package main

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"

	laclopenai "github.com/cloudwego/eino-ext/libs/acl/openai"
	gc "github.com/rthornton128/goncurses"

	"github.com/mikeb26/gptcli/internal"
	"github.com/mikeb26/gptcli/internal/am"
	"github.com/mikeb26/gptcli/internal/scm"
	"github.com/mikeb26/gptcli/internal/scm/git"
	"github.com/mikeb26/gptcli/internal/threads"
	"github.com/mikeb26/gptcli/internal/types"
	"github.com/mikeb26/gptcli/internal/ui"
)

const (
	CommandName            = "gptcli"
	KeyFileFmt             = ".%v.key"
	PrefsFile              = "prefs.json"
	ApprovePolicyFile      = "approvals.json"
	ThreadsDirOld          = "threads"
	ArchiveDirOld          = "archive_threads"
	ThreadGroupsDir        = "thread_groups"
	LogsDir                = "logs"
	AuditLogFile           = "audit.log"
	MainThreadGroupName    = "main"
	ArchiveThreadGroupName = "archive"
)

type Prefs struct {
	SummarizePrior bool   `json:"summarize_prior"`
	Vendor         string `json:"vendor"`
	Model          string `json:"model"`
	EnableAuditLog bool   `json:"enable_audit_log"`
}

type Toggles struct {
	summary    bool
	useColors  bool
	needConfig bool
}

type CliContext struct {
	ictx types.InternalContext

	ui          *ui.NcursesUI
	rootWin     *gc.Window
	menu        *threadMenuUI
	threadViews map[string]*threadViewUI

	prefs   Prefs
	toggles Toggles

	threadGroupSet *threads.ThreadGroupSet
	curThreadGroup string

	scmClient scm.Client
}

func NewCliContext(ctx context.Context) (*CliContext, error) {
	var err error
	var rootWinLocal *gc.Window
	rootWinLocal, err = gcInit()
	if err != nil {
		return nil, err
	}

	vendor := internal.DefaultVendor
	model := internal.GetVendorInfo(vendor).DefaultModel
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
			Model:          model,
			EnableAuditLog: true,
		},
		threadGroupSet: nil,
		threadViews:    make(map[string]*threadViewUI),
		curThreadGroup: MainThreadGroupName,
		scmClient:      git.NewClient(),
	}
	cliCtx.menu = newThreadMenuUI(cliCtx)

	threadGroupsDirLocal, err := getThreadGroupsDir()
	if err != nil {
		threadGroupsDirLocal = "/tmp"
	}

	cliCtx.threadGroupSet = threads.NewThreadGroupSet(threadGroupsDirLocal,
		[]string{MainThreadGroupName, ArchiveThreadGroupName})

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
	cliCtx.ictx.LlmModel = cliCtx.prefs.Model
	cliCtx.ictx.LlmApiKey = keyText
	cliCtx.ictx.LlmReasoningEffort = laclopenai.ReasoningEffortLevelMedium
	if cliCtx.prefs.EnableAuditLog {
		cliCtx.ictx.LlmAuditLogPath = auditLogPath
	}
	cliCtx.ictx.LlmBaseApprover = ui.NewUIApprover(cliCtx.ui)

	err = cliCtx.threadGroupSet.Load()
	if err != nil {
		return err
	}
	cliCtx.toggles.needConfig = false

	return nil
}

func main() {
	ctx := context.Background()
	cliCtx, err := NewCliContext(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v: Failed to initialize UI: %v\n", CommandName,
			err)
		os.Exit(1)
	}

	err = cliCtx.migrateOldThreadGroupFomatIfNeeded()
	if err != nil {
		gcExit()
		fmt.Fprintf(os.Stderr, "%v: Failed to migrate existing threads to new format: %v\n", CommandName, err)
		os.Exit(1)
	}

	err = cliCtx.load(ctx)
	if err != nil && !cliCtx.toggles.needConfig {
		gcExit()
		fmt.Fprintf(os.Stderr, "%v: Failed to load: %v\n", CommandName, err)
		os.Exit(1)
	}

	err = showMenu(ctx, cliCtx)
	gcExit()

	if err == io.EOF {
		fmt.Fprintf(os.Stderr, "Upgrade complete. Please restart.")
		os.Exit(1)
	}
}
