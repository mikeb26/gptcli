/* Copyright © 2025-2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/mikeb26/gptcli/internal/threads"
)

const (
	RowFmt    = "│ %8v │ %8v │ %18v │ %18v │ %18v │ %-18v\n"
	RowSpacer = "──────────────────────────────────────────────────────────────────────────────────────────────\n"
)

func threadHeaderString(t threads.Thread) string {
	now := time.Now()

	aTime := formatHeaderTime(t.AccessTime(), now)
	mTime := formatHeaderTime(t.ModTime(), now)
	cTime := formatHeaderTime(t.CreateTime(), now)

	// Append "*" when the thread needs user attention.
	//
	// 1) If the thread is blocked awaiting approval, we want the thread list
	//    (including the ncurses menu) to visually flag it immediately, even
	//    though no dialogue has been persisted yet.
	// 2) If the thread has been modified since it was last accessed, also flag it.
	stateSuffix := ""
	if t.State() == threads.ThreadStateBlocked || t.ModTime().After(t.AccessTime()) {
		stateSuffix = "*"
	}

	return fmt.Sprintf(RowFmt, t.Id(), t.State().String()+stateSuffix,
		aTime, mTime, cTime, t.Name())
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

func threadGroupHeaderString(includeSpacers bool) string {
	var sb strings.Builder

	if includeSpacers {
		sb.WriteString(RowSpacer)
	}
	sb.WriteString(fmt.Sprintf(RowFmt, "Thread#", "State", "Last Accessed",
		"Last Modified", "Created", "Name"))

	if includeSpacers {
		sb.WriteString(RowSpacer)
	}

	return sb.String()
}

func threadGroupFooterString() string {
	return RowSpacer
}

func threadGroupString(thrGrp *threads.ThreadGroup, header bool,
	footer bool) string {

	var sb strings.Builder

	if header {
		sb.WriteString(threadGroupHeaderString(true))
	}

	for _, t := range thrGrp.Threads() {
		sb.WriteString(threadHeaderString(t))
	}

	if footer {
		sb.WriteString(threadGroupFooterString())
	}

	return sb.String()
}

func (cliCtx *CliContext) isCurArchived() bool {
	return cliCtx.curThreadGroup == ArchiveThreadGroupName
}

func (cliCtx *CliContext) migrateOldThreadGroupFomatIfNeeded() error {
	oldMainDir, err := getThreadsDirOld()
	if err != nil {
		return err
	}
	oldArchiveDir, err := getArchiveDirOld()
	if err != nil {
		return err
	}
	thrGrpDir, err := getThreadGroupsDir()
	if err != nil {
		return err
	}

	err = cliCtx.migrateOneOldThreadGroupFormat(oldMainDir, thrGrpDir,
		MainThreadGroupName)
	if err != nil {
		return err
	}
	return cliCtx.migrateOneOldThreadGroupFormat(oldArchiveDir, thrGrpDir,
		ArchiveThreadGroupName)
}

func (cliCtx *CliContext) migrateOneOldThreadGroupFormat(oldDir string,
	thrGrpDir string, thrGrpName string) error {

	dEntries, err := os.ReadDir(oldDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, dEnt := range dEntries {
		err = cliCtx.migrateOneOldThreadFormat(dEnt, oldDir, thrGrpDir,
			thrGrpName)
		if err != nil {
			return err
		}
	}

	return os.RemoveAll(oldDir)
}

func (cliCtx *CliContext) migrateOneOldThreadFormat(dEntry os.DirEntry,
	oldDir string, thrGrpDir string, thrGrpName string) error {

	oldThreadFile := filepath.Join(oldDir, dEntry.Name())
	newThreadDir := strings.TrimSuffix(dEntry.Name(), path.Ext(dEntry.Name()))
	newThreadDir = filepath.Join(thrGrpDir, thrGrpName, newThreadDir)
	newThreadFile := filepath.Join(newThreadDir, threads.ThreadFileName)

	content, err := os.ReadFile(oldThreadFile)
	if err != nil {
		return err
	}
	err = os.MkdirAll(newThreadDir, 0700)
	if err != nil {
		return err
	}
	err = os.WriteFile(newThreadFile, content, 0600)
	if err != nil {
		return err
	}

	return nil
}
