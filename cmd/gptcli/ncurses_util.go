/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package main

import (
	"fmt"
	"os"

	gc "github.com/gbin/goncurses"
	"golang.org/x/term"

	"github.com/mikeb26/gptcli/internal/types"
	"github.com/mikeb26/gptcli/internal/ui"
)

// statusSegment represents a slice of text within a status bar and
// whether it should be highlighted as a key (bold / different color).
type statusSegment struct {
	text string
	bold bool
}

// drawStatusSegments renders a status bar composed of the provided
// segments on the given row. It applies a uniform background (reverse
// video or the menuColorStatus pair) and highlights bold segments using
// either A_BOLD or the menuColorStatusKey pair when colors are active.
func drawStatusSegments(scr *gc.Window, y, maxX int, segments []statusSegment, useColors bool) {
	if y < 0 {
		return
	}

	var baseAttr gc.Char = gc.A_REVERSE
	if useColors {
		baseAttr = gc.ColorPair(menuColorStatus)
	}
	_ = scr.AttrSet(baseAttr)
	scr.Move(y, 0)
	scr.HLine(y, 0, ' ', maxX)

	x := 0
	for _, seg := range segments {
		if x >= maxX {
			break
		}
		if seg.bold {
			if useColors {
				_ = scr.AttrSet(gc.ColorPair(menuColorStatusKey))
			} else {
				_ = scr.AttrOn(gc.A_BOLD)
			}
		} else {
			_ = scr.AttrSet(baseAttr)
		}

		remaining := maxX - x
		if remaining <= 0 {
			break
		}
		runes := []rune(seg.text)
		if len(runes) > remaining {
			runes = runes[:remaining]
		}
		text := string(runes)

		scr.MovePrint(y, x, text)
		x += len(runes)
	}
}

// promptForThreadNameNCurses displays a simple centered modal window asking
// the user to enter a new thread name. It returns the entered string (with
// surrounding whitespace trimmed) or an empty string if the user cancels
// with ESC. All interaction happens via ncurses so it is safe to call while
// the main menu UI is active.
func promptForThreadNameNCurses(nui *ui.NcursesUI) (string, error) {
	// Delegate to the shared NcursesUI helper so we don't duplicate
	// modal input handling. ESC is treated as cancellation and mapped to
	// an empty string by NcursesUI.Get.
	name, err := nui.Get("Enter new thread name (ESC to cancel):")
	if err != nil {
		return "", err
	}

	return name, nil
}

// showErrorRetryModal displays a simple yes/no prompt using NcursesUI
// and returns true if the user chooses to retry. The prompt includes
// the error message followed by "Retry? (y/n)". ESC or an empty
// response are treated the same as selecting "n" (do not retry).
func showErrorRetryModal(nui *ui.NcursesUI, message string) (bool, error) {
	// Build a compact prompt that shows the error text and asks whether
	// to retry. NcursesUI.SelectBool handles rendering the modal and
	// collecting the response.
	prompt := fmt.Sprintf("Error: %s\nRetry? (y/n)", message)
	trueOpt := types.GptCliUIOption{Key: "y", Label: "y"}
	falseOpt := types.GptCliUIOption{Key: "n", Label: "n"}
	defaultNo := false

	return nui.SelectBool(prompt, trueOpt, falseOpt, &defaultNo)
}

// Helper to synchronize ncurses' idea of the terminal size with the
// actual TTY size. This uses golang.org/x/term to query the real
// dimensions, then asks ncurses (via goncurses) to resize its
// internal structures. All ncurses calls stay on this goroutine to
// avoid concurrency issues with C state.
func resizeScreen(scr *gc.Window) {
	cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return
	}

	if !gc.IsTermResized(rows, cols) {
		return
	}

	_ = gc.ResizeTerm(rows, cols)
}
