/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package ui

import (
	"fmt"
	"strings"
	"sync"

	gc "github.com/gbin/goncurses"
	"github.com/mikeb26/gptcli/internal/types"
)

// NcursesUI implements the UI interface using a goncurses Window.
//
// It is designed to be used from code that has already initialized
// ncurses via goncurses.Init() and obtained the root screen/window. The
// lifecycle of ncurses (Init/End) is intentionally left to the caller so
// that NcursesUI can be embedded into existing TUI flows such as the
// thread menu and thread view.
//
// All public methods are safe for concurrent use; calls are
// serialized with a mutex to avoid interleaving prompts.
type NcursesUI struct {
	mu  sync.Mutex
	scr *gc.Window

	theme Theme
}

// NewNcursesUI wraps an existing ncurses screen/window. The caller is
// responsible for having called goncurses.Init() and for calling
// goncurses.End() when the application is finished with ncurses.
func NewNcursesUI(scrIn *gc.Window) *NcursesUI {
	if scrIn == nil {
		panic("non-nil screen required to init ncursesui")
	}

	_ = scrIn.Keypad(true)

	return &NcursesUI{
		scr:   scrIn,
		theme: Theme{},
	}
}

// SetTheme sets styling preferences for NcursesUI widgets (such as list
// selection modals).
//
// Callers should set this after initializing ncurses color pairs.
func (n *NcursesUI) SetTheme(theme Theme) {
	if n == nil {
		return
	}
	n.theme = theme
}

// TruncateRunes returns a prefix of s that fits in max runes. It is
// safe for UTF-8 strings because it operates on runes rather than
// bytes, and assumes (for our use-cases) that each rune occupies a
// single column cell.
func TruncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// SelectOption presents a list of options to the user in a centered
// ncurses modal and reads their selection. The user navigates with the
// arrow keys (plus PgUp/PgDn/Home/End for larger lists) and presses Enter
// to choose the highlighted option. Pressing ESC cancels the dialog and
// returns an error.
func (n *NcursesUI) SelectOption(userPrompt string,
	choices []types.UIOption) (types.UIOption, error) {

	n.mu.Lock()
	defer n.mu.Unlock()

	if len(choices) == 0 {
		return types.UIOption{}, fmt.Errorf("no choices provided")
	}

	optionLines := make([]string, len(choices))
	for i, c := range choices {
		optionLines[i] = c.Label
	}

	idx, canceled, err := n.selectFromListModalFrame(userPrompt, optionLines, 0)
	if err != nil {
		return types.UIOption{}, err
	}
	if canceled {
		return types.UIOption{}, fmt.Errorf("selection cancelled")
	}
	return choices[idx], nil
}

// SelectBool presents a true and false option to the user via a list
// selection modal and returns their choice. The user navigates with the
// arrow keys and presses Enter to choose the highlighted option. The
// initial highlight reflects defaultOpt when provided. ESC preserves the
// previous semantics from the line-based implementation: with a default,
// ESC selects the default; without a default, ESC is treated as an
// invalid selection and the user is re-prompted with an error prefix.
func (n *NcursesUI) SelectBool(userPrompt string,
	trueOption, falseOption types.UIOption,
	defaultOpt *bool) (bool, error) {

	n.mu.Lock()
	defer n.mu.Unlock()

	items := []string{trueOption.Label, falseOption.Label}
	initialSelected := 0
	if defaultOpt != nil && !*defaultOpt {
		initialSelected = 1
	}

	prompt := strings.TrimRight(userPrompt, "\n")

	for {
		// If the prompt includes very long lines (common for error messages),
		// use the scrollable modal variant so the user can view the entire
		// message instead of it being truncated.
		useScrollable := false
		if n != nil && n.scr != nil {
			_, maxX := n.scr.MaxYX()
			contentTextWidth := maxX - 8
			if contentTextWidth < 20 {
				contentTextWidth = 20
			}
			lines := strings.Split(strings.TrimRight(prompt, "\n"), "\n")
			for _, line := range lines {
				if len([]rune(line)) > contentTextWidth {
					useScrollable = true
					break
				}
			}
		}

		if useScrollable {
			choice, canceled, err := n.selectBoolScrollablePromptModalFrame(prompt, trueOption, falseOption, defaultOpt)
			if err != nil {
				return false, err
			}
			if !canceled {
				return choice, nil
			}
		} else {
			idx, canceled, err := n.selectFromListModalFrame(prompt, items, initialSelected)
			if err != nil {
				return false, err
			}
			if !canceled {
				// Enter pressed: return the highlighted choice.
				return idx == 0, nil
			}
		}

		// ESC pressed: preserve prior semantics.
		if defaultOpt != nil {
			// Previously, ESC produced an empty input line which selected
			// the default when provided.
			return *defaultOpt, nil
		}

		// Without a default, ESC is treated as an invalid selection, which
		// triggers a re-prompt with an "Invalid selection." prefix.
		prompt = "Invalid selection. " + userPrompt
	}
}

// Get prompts the user for a line of input and returns it, stripping the
// trailing newline. The prompt is displayed in a centered ncurses modal.
func (n *NcursesUI) Get(userPrompt string) (string, error) {

	n.mu.Lock()
	defer n.mu.Unlock()

	line, err := n.readLineModalFrame(userPrompt)
	if err != nil {
		return "", err
	}

	line = strings.TrimRight(line, "\r\n")
	line = strings.TrimSpace(line)
	return line, nil
}

// Confirm shows a modal dialog with the provided prompt and an "OK" label.
// It blocks until the user presses Enter.
func (n *NcursesUI) Confirm(userPrompt string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	trimmed := strings.TrimRight(userPrompt, "\n")
	promptLines := strings.Split(trimmed, "\n")
	if len(promptLines) == 0 {
		promptLines = []string{""}
	}

	maxRunes := 0
	for _, line := range promptLines {
		if l := len([]rune(line)); l > maxRunes {
			maxRunes = l
		}
	}
	if okLen := len([]rune("OK")); okLen > maxRunes {
		maxRunes = okLen
	}

	innerWidth := maxRunes + 2
	if innerWidth < 30 {
		innerWidth = 30
	}

	// Border + prompt lines + spacer + OK line.
	desiredHeight := len(promptLines) + 4
	if desiredHeight < 6 {
		desiredHeight = 6
	}

	modal, err := n.newCenteredModal(desiredHeight, innerWidth, false /* hasCursor */, false /* hasInput */)
	if err != nil {
		return err
	}
	defer modal.Close()

	win := modal.Frame.Win
	cy, cx, ch, cw := modal.ContentArea()
	if cw < 1 {
		cw = 1
	}
	if ch < 1 {
		ch = 1
	}

	win.Timeout(50)
	defer win.Timeout(-1)

	// Draw the border once; subsequent updates only modify the inner area.
	_ = win.Box(0, 0)

	_ = win.AttrSet(n.theme.NormalAttr())
	for i, line := range promptLines {
		if i >= ch {
			break
		}
		printClipped(win, cy+i, cx, cw, line)
	}

	okY := cy + len(promptLines) + 1
	if okY >= cy+ch {
		okY = cy + ch - 1
	}
	win.Move(okY, cx)
	win.HLine(okY, cx, ' ', cw)
	printClipped(win, okY, cx, cw, "OK")
	win.Refresh()

	for {
		chKey := win.GetChar()
		if chKey == 0 {
			continue
		}
		switch chKey {
		case gc.KEY_ENTER, gc.KEY_RETURN:
			return nil
		default:
			// Ignore other keys.
		}
	}
}
