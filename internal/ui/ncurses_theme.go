/* Copyright © 2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package ui

import gc "github.com/gbin/goncurses"

// Theme configures optional styling for NcursesUI widgets.
//
// Theme values are intentionally minimal; they allow cmd/gptcli to keep
// color-pair ownership and initialization centralized while internal/ui
// widgets can still render consistently.
type Theme struct {
	// UseColors indicates whether the caller successfully initialized
	// ncurses colors via StartColor()/InitPair.
	UseColors bool

	// SelectedPair is the ncurses color pair ID to use for selected list
	// items.
	SelectedPair int16
}

// SelectedAttr returns the attribute to use for selected list items.
//
// If colors are enabled (UseColors) and SelectedPair is set, it uses the
// configured color pair. Otherwise it falls back to reverse video.
func (t Theme) SelectedAttr() gc.Char {
	if t.UseColors && t.SelectedPair != 0 {
		return gc.A_NORMAL | gc.ColorPair(t.SelectedPair)
	}
	return gc.A_REVERSE | gc.A_NORMAL
}

// NormalAttr returns the default attribute for non-selected content.
func (t Theme) NormalAttr() gc.Char { return gc.A_NORMAL }
