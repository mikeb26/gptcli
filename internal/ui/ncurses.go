package ui

import (
	"fmt"
	"strings"
	"sync"

	gc "github.com/gbin/goncurses"
	"github.com/mikeb26/gptcli/internal/types"
)

// uiColorSelected mirrors the menuColorSelected color pair used by the
// main ncurses thread menu (see cmd/gptcli/ncurses_menu.go). We rely on
// that code having initialized this color pair via gc.InitPair before our
// dialogs are shown. Using the same numeric pair keeps the selection
// styling consistent (cyan background) across the menu and modal dialogs.
const uiColorSelected int16 = 3

// NcursesUI implements the GptCliUI interface using a goncurses Window.
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
}

// NewNcursesUI wraps an existing ncurses screen/window. The caller is
// responsible for having called goncurses.Init() and for calling
// goncurses.End() when the application is finished with ncurses.
func NewNcursesUI(scrIn *gc.Window) *NcursesUI {
	if scrIn == nil {
		panic("non-nil screen required to init ncursesui")
	}

	_ = scrIn.Keypad(true)

	return &NcursesUI{scr: scrIn}
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

// newCenteredBox creates a boxed ncurses window of approximately the
// requested height and width, centered on the screen. The dimensions
// are clamped to fit within the current terminal size and to a
// reasonable minimum so that there is always room for borders and at
// least one row/column of content. It returns the window along with the
// inner content width and height (excluding borders).
func (n *NcursesUI) newCenteredBox(height, width int) (*gc.Window, int, int, error) {
	maxY, maxX := n.scr.MaxYX()
	if maxY < 3 || maxX < 4 {
		return nil, 0, 0, fmt.Errorf("terminal too small for ncurses window")
	}

	if height < 3 {
		height = 3
	}
	if height > maxY-2 {
		height = maxY - 2
	}
	if width < 4 {
		width = 4
	}
	if width > maxX-2 {
		width = maxX - 2
	}
	if height < 3 || width < 4 {
		return nil, 0, 0, fmt.Errorf("terminal too small for requested window")
	}

	startY := (maxY - height) / 2
	startX := (maxX - width) / 2
	if startY < 0 {
		startY = 0
	}
	if startX < 0 {
		startX = 0
	}

	win, err := gc.NewWindow(height, width, startY, startX)
	if err != nil {
		return nil, 0, 0, err
	}
	_ = win.Keypad(true)
	_ = win.Box(0, 0)

	innerW := width - 2
	if innerW < 1 {
		innerW = 1
	}
	innerH := height - 2
	if innerH < 1 {
		innerH = 1
	}

	return win, innerW, innerH, nil
}

// Delete the modal window and force the root screen to be treated as
// fully "touched" so that a subsequent Refresh repaintes the entire
// area the dialog occupied. This prevents modal artifacts from being
// left on the physical terminal.
func deleteModelAndRefreshParent(modal, parent *gc.Window) {
	_ = modal.Delete()
	_ = parent.Touch()
	parent.Refresh()
}

// readLineModal displays a simple centered modal window with the provided
// prompt and returns the line of user input (without a trailing newline).
func (n *NcursesUI) readLineModal(userPrompt string) (string, error) {
	// Allow multi-line prompts by splitting on explicit newlines. This keeps
	// sizing and rendering consistent with the actual on-screen layout so
	// that we never write outside the inner content box.
	trimmed := strings.TrimRight(userPrompt, "\n")
	promptLines := strings.Split(trimmed, "\n")

	// Basic modal dimensions: borders + prompt lines + input line.
	desiredHeight := len(promptLines) + 3
	if desiredHeight < 5 {
		desiredHeight = 5
	}

	// Width is based on the longest prompt line.
	maxRunes := 0
	for _, line := range promptLines {
		if l := len([]rune(line)); l > maxRunes {
			maxRunes = l
		}
	}
	innerWidth := maxRunes + 2
	if innerWidth < 30 {
		innerWidth = 30
	}
	win, contentWidth, contentHeight, err := n.newCenteredBox(desiredHeight, innerWidth)
	if err != nil {
		return "", err
	}
	defer deleteModelAndRefreshParent(win, n.scr)
	if contentWidth < 1 {
		contentWidth = 1
	}
	if contentHeight < 1 {
		contentHeight = 1
	}

	// Render each prompt line starting at the first inner row. Any excess
	// lines are silently dropped if the terminal is extremely small.
	for i, line := range promptLines {
		if 1+i > contentHeight {
			break
		}
		win.MovePrint(1+i, 1, TruncateRunes(line, contentWidth))
	}

	var buf []rune
	// Place the input row directly after the last rendered prompt line,
	// clamped to the last available inner row so we stay inside the box.
	inputY := 1 + len(promptLines)
	if inputY > contentHeight {
		inputY = contentHeight
	}
	for {
		// Clear input line inside the box area.
		for x := 1; x < contentWidth+1; x++ {
			win.MovePrint(inputY, x, " ")
		}

		// Render current buffer truncated to fit.
		inputWidth := contentWidth
		if inputWidth < 1 {
			inputWidth = 1
		}
		text := TruncateRunes(string(buf), inputWidth)
		win.MovePrint(inputY, 1, text)

		cursorX := 1 + len([]rune(text))
		if cursorX >= contentWidth+1 {
			cursorX = contentWidth
		}
		win.Move(inputY, cursorX)
		win.Refresh()

		ch := win.GetChar()
		if ch == 0 {
			continue
		}

		switch ch {
		case gc.Key(27): // ESC -> empty string
			return "", nil
		case gc.KEY_ENTER, gc.KEY_RETURN:
			return string(buf), nil
		case gc.KEY_BACKSPACE, 127, 8:
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
			}
		default:
			// Accept any non-control byte (including high-bit bytes from
			// UTF-8 sequences) as literal input. goncurses/wgetch reports
			// regular characters as values in the 0-255 range and uses
			// larger values for KEY_* constants, so we clamp at 256 to
			// avoid accidentally treating special keys as text.
			if ch >= 32 && ch < 256 {
				buf = append(buf, rune(ch))
			}
		}
	}
}

// selectFromListModal displays a centered, scrollable list with the given
// prompt. The user navigates with arrow keys and related movement keys and
// presses Enter to select the highlighted item. If the user presses ESC the
// selection is reported as canceled. The caller can decide how to interpret
// cancellation (e.g. treating it as default selection).
func (n *NcursesUI) selectFromListModal(userPrompt string,
	items []string,
	initialSelected int) (selectedIdx int, canceled bool, err error) {

	if len(items) == 0 {
		return -1, false, fmt.Errorf("no items provided")
	}

	// Total number of items, used both for sizing calculations and for
	// selection/scrolling logic.
	total := len(items)

	// Allow multi-line prompts and size the modal based on the actual
	// rendered lines plus the list content.
	trimmed := strings.TrimRight(userPrompt, "\n")
	promptLines := strings.Split(trimmed, "\n")
	if len(promptLines) == 0 {
		promptLines = []string{""}
	}
	promptHeight := len(promptLines)

	maxRunes := 0
	for _, line := range promptLines {
		if l := len([]rune(line)); l > maxRunes {
			maxRunes = l
		}
	}
	for _, it := range items {
		if l := len([]rune(it)); l > maxRunes {
			maxRunes = l
		}
	}

	innerWidth := maxRunes + 2
	if innerWidth < 30 {
		innerWidth = 30
	}
	// Borders + prompt lines + a blank spacer row + at least one row for
	// items. The blank row visually separates the title from the list.
	desiredHeight := promptHeight + 1 + len(items) + 2
	if desiredHeight < promptHeight+3 {
		desiredHeight = promptHeight + 3
	}

	win, contentWidth, contentHeight, err := n.newCenteredBox(desiredHeight, innerWidth)
	if err != nil {
		return -1, false, err
	}
	defer deleteModelAndRefreshParent(win, n.scr)
	if contentWidth < 1 {
		contentWidth = 1
	}
	if contentHeight < 1 {
		contentHeight = 1
	}

	// If the prompt is so tall relative to the actual window height that it
	// would push the list completely off-screen, trim the number of visible
	// prompt lines so that we always reserve at least a handful of rows for
	// the selectable list. This prevents the options from becoming
	// invisible on small terminals or with very long prompts.
	//
	// When the number of items is smaller than our minimum reserved list
	// rows, we reduce the reservation to match the actual item count so we
	// don't end up with extra blank rows in tiny lists (e.g. a boolean
	// selector with just two options).
	const minListRows = 3
	effectiveMinListRows := minListRows
	if total < effectiveMinListRows {
		effectiveMinListRows = total
	}
	maxPromptLines := contentHeight - (effectiveMinListRows + 1) // +1 for blank spacer
	if maxPromptLines < 1 {
		// Ensure at least one visible prompt line even when the terminal is
		// extremely small. In that case we may not be able to reserve the
		// full minListRows, but we will still leave as much space as
		// possible for the list.
		maxPromptLines = 1
	}
	if promptHeight > maxPromptLines {
		promptHeight = maxPromptLines
		promptLines = promptLines[:promptHeight]
	}

	// Selection / scrolling state. This mirrors the behavior of the main
	// thread menu: we maintain a selected index and a top-of-window offset
	// into the full list.
	selected := initialSelected
	if selected < 0 {
		selected = 0
	}
	if selected >= total {
		selected = total - 1
	}
	offset := 0

	adjust := func(viewHeight int) {
		if viewHeight <= 0 || total == 0 {
			// Nothing meaningful to show; clamp indices.
			offset = 0
			if total == 0 {
				selected = 0
			} else if selected >= total {
				selected = total - 1
			} else if selected < 0 {
				selected = 0
			}
			return
		}

		if selected < 0 {
			selected = 0
		}
		if selected >= total {
			selected = total - 1
		}

		if offset > selected {
			offset = selected
		}
		if selected >= offset+viewHeight {
			offset = selected - viewHeight + 1
		}

		maxOffset := total - viewHeight
		if maxOffset < 0 {
			maxOffset = 0
		}
		if offset > maxOffset {
			offset = maxOffset
		}
		if offset < 0 {
			offset = 0
		}
	}

	for {
		// Recompute view height each iteration to respect very small
		// terminals. The list area starts after the prompt lines and a
		// single blank spacer row, and occupies the remaining inner rows.
		viewHeight := contentHeight - (promptHeight + 1)
		if viewHeight < 1 {
			viewHeight = 1
		}
		adjust(viewHeight)

		// Clear content area. Always reset attributes to normal before
		// clearing so we do not accidentally paint the prompt region or
		// spacer line with the selection color from a previous iteration.
		_ = win.AttrSet(gc.A_NORMAL)
		for y := 1; y <= contentHeight; y++ {
			win.Move(y, 1)
			win.HLine(y, 1, ' ', contentWidth)
		}

		// Render prompt lines at the top.
		_ = win.AttrSet(gc.A_NORMAL)
		for i, line := range promptLines {
			if 1+i > contentHeight {
				break
			}
			win.MovePrint(1+i, 1, TruncateRunes(line, contentWidth))
		}

		// Render list items within the remaining rows. The selected item is
		// highlighted using the same cyan background as the main menu
		// (color pair uiColorSelected) when colors are available, and
		// reverse video otherwise. We always fill the entire row so the
		// highlight spans the full width of the list area.
		selectedAttr := gc.A_NORMAL | gc.ColorPair(uiColorSelected)
		normalAttr := gc.A_NORMAL
		listStartY := 1 + promptHeight + 1 // one blank row after prompt
		for row := 0; row < viewHeight; row++ {
			idx := offset + row
			y := listStartY + row
			if y > contentHeight {
				break
			}
			if idx >= total {
				// No more items; leave the rest of the area blank.
				continue
			}

			text := TruncateRunes(items[idx], contentWidth)
			if idx == selected {
				_ = win.AttrSet(selectedAttr)
			} else {
				_ = win.AttrSet(normalAttr)
			}
			// Fill the entire inner row so the selection highlight spans the
			// full width, then render the text at the start of the line.
			win.Move(y, 1)
			win.HLine(y, 1, ' ', contentWidth)
			win.MovePrint(y, 1, text)
		}

		win.Refresh()

		ch := win.GetChar()
		if ch == 0 {
			continue
		}

		switch ch {
		case gc.Key(27): // ESC -> report cancellation
			return -1, true, nil
		case gc.KEY_ENTER, gc.KEY_RETURN:
			return selected, false, nil
		case gc.KEY_UP:
			if selected > 0 {
				selected--
			}
		case gc.KEY_DOWN:
			if selected < total-1 {
				selected++
			}
		case gc.KEY_HOME:
			selected = 0
		case gc.KEY_END:
			selected = total - 1
		case gc.KEY_PAGEUP:
			if viewHeight > 0 {
				selected -= viewHeight
				if selected < 0 {
					selected = 0
				}
			}
		case gc.KEY_PAGEDOWN:
			if viewHeight > 0 {
				selected += viewHeight
				if selected > total-1 {
					selected = total - 1
				}
			}
		default:
			// Ignore other keys; no direct numeric entry for now.
		}
	}
}

// SelectOption presents a list of options to the user in a centered
// ncurses modal and reads their selection. The user navigates with the
// arrow keys (plus PgUp/PgDn/Home/End for larger lists) and presses Enter
// to choose the highlighted option. Pressing ESC cancels the dialog and
// returns an error.
func (n *NcursesUI) SelectOption(userPrompt string,
	choices []types.GptCliUIOption) (types.GptCliUIOption, error) {

	n.mu.Lock()
	defer n.mu.Unlock()

	if len(choices) == 0 {
		return types.GptCliUIOption{}, fmt.Errorf("no choices provided")
	}

	optionLines := make([]string, len(choices))
	for i, c := range choices {
		optionLines[i] = c.Label
	}

	idx, canceled, err := n.selectFromListModal(userPrompt, optionLines, 0)
	if err != nil {
		return types.GptCliUIOption{}, err
	}
	if canceled {
		return types.GptCliUIOption{}, fmt.Errorf("selection cancelled")
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
	trueOption, falseOption types.GptCliUIOption,
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
		idx, canceled, err := n.selectFromListModal(prompt, items, initialSelected)
		if err != nil {
			return false, err
		}

		if !canceled {
			// Enter pressed: return the highlighted choice.
			return idx == 0, nil
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

	line, err := n.readLineModal(userPrompt)
	if err != nil {
		return "", err
	}

	line = strings.TrimRight(line, "\r\n")
	line = strings.TrimSpace(line)
	return line, nil
}
