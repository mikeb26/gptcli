/* Copyright © 2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package ui

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
