/* Copyright © 2025-2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package main

import (
	"slices"
	"strings"

	"github.com/mikeb26/gptcli/internal/threads"
	"github.com/mikeb26/gptcli/internal/types"
)

func threadContainsSearchStr(t threads.Thread, searchStr string) bool {
	if strings.Contains(t.Name(), searchStr) {
		return true
	}

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

func (ui *threadMenuUI) isSearchActive() bool {
	return ui.searchQuery != ""
}

func (ui *threadMenuUI) promptForSearchQuery() (string, error) {
	q, err := ui.cliCtx.ui.Get("Search threads (case-sensitive, ESC to cancel):")
	if err != nil {
		return "", err
	}
	// NcursesUI.Get returns "" on ESC; treat it as cancel.
	if q == "" {
		return "", nil
	}
	return q, nil
}

func (ui *threadMenuUI) buildSearchEntries(query string) []threadMenuEntry {
	cliCtx := ui.cliCtx

	entries := make([]threadMenuEntry, 0)
	thrGroups := []string{MainThreadGroupName, ArchiveThreadGroupName}
	for _, thrGrp := range thrGroups {
		for _, t := range cliCtx.threadGroupSet.Threads([]string{thrGrp}) {
			if !threadContainsSearchStr(t, query) {
				continue
			}

			line := strings.TrimRight(threadHeaderString(t), "\n")
			entries = append(entries, threadMenuEntry{
				label:      line,
				thread:     t,
				isArchived: thrGrp == ArchiveThreadGroupName,
			})
		}
	}

	slices.SortFunc(entries, func(a, b threadMenuEntry) int {
		return -a.thread.AccessTime().Compare(b.thread.AccessTime())
	})

	return entries
}

func (ui *threadMenuUI) doSearch(query string) {
	ui.searchQuery = query
	ui.selected = 0
	ui.offset = 0
	ui.entries = ui.buildSearchEntries(query)
}

func (ui *threadMenuUI) clearSearch() {
	ui.searchQuery = ""
	ui.selected = 0
	ui.offset = 0
}
