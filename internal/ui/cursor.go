/* Copyright © 2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package ui

import (
	"sync/atomic"

	gc "github.com/rthornton128/goncurses"
)

// cursorVisible tracks the last cursor visibility value we attempted to
// set via SetCursorVisible.
//
// Ncurses does not expose a reliable query for the current cursor
// visibility. goncurses' Cursor() only returns an error, so we keep a
// best-effort shadow state and require code in this repo to call
// SetCursorVisible instead of gc.Cursor directly.
var cursorVisible atomic.Bool

func init() {
	cursorVisible.Store(false)
}

// SetCursorVisible sets ncurses terminal cursor visibility.
//
// This is global ncurses state; callers should treat it as a process-wide
// setting.
func SetCursorVisible(visible bool) {
	cursorVisible.Store(visible)
	if visible {
		_ = gc.Cursor(1)
		return
	}
	_ = gc.Cursor(0)
}

// CursorVisible returns the last cursor visibility value set via
// SetCursorVisible.
func CursorVisible() bool {
	return cursorVisible.Load()
}
