package ui

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	gc "github.com/gbin/goncurses"
	"github.com/mikeb26/gptcli/internal/types"
)

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

// SelectOption presents a list of options to the user in a centered
// ncurses modal and reads their selection. The user can type the numeric
// index of the desired option and press Enter. It returns an error if the
// input is invalid or if the underlying screen is unavailable.
func (n *NcursesUI) SelectOption(userPrompt string,
	choices []types.GptCliUIOption) (types.GptCliUIOption, error) {

	n.mu.Lock()
	defer n.mu.Unlock()

	if len(choices) == 0 {
		return types.GptCliUIOption{}, fmt.Errorf("no choices provided")
	}
	prompt := strings.TrimRight(userPrompt, "\n")
	promptRunes := []rune(prompt)

	optionLines := make([]string, len(choices))
	maxLineLen := len(promptRunes)
	for i, c := range choices {
		line := fmt.Sprintf("%d) %s", i+1, c.Label)
		optionLines[i] = line
		if l := len([]rune(line)); l > maxLineLen {
			maxLineLen = l
		}
	}

	// Height: borders + prompt + choices + input line.
	desiredHeight := len(choices) + 4
	if desiredHeight < 4 {
		desiredHeight = 4
	}
	innerWidth := maxLineLen + 2
	if innerWidth < 30 {
		innerWidth = 30
	}
	win, contentWidth, contentHeight, err := n.newCenteredBox(desiredHeight, innerWidth)
	if err != nil {
		return types.GptCliUIOption{}, err
	}
	defer deleteModelAndRefreshParent(win, n.scr)
	if contentWidth < 1 {
		contentWidth = 1
	}

	// Prompt.
	win.MovePrint(1, 1, TruncateRunes(prompt, contentWidth))

	// Options (may be truncated vertically if the terminal is very small).
	maxOptionsVisible := contentHeight - 2 // minus prompt and input line
	if maxOptionsVisible < 0 {
		maxOptionsVisible = 0
	}
	for i := 0; i < len(optionLines) && i < maxOptionsVisible; i++ {
		win.MovePrint(2+i, 1, TruncateRunes(optionLines[i], contentWidth))
	}

	// Input line on the last inner row of the content area (just above
	// the bottom border). contentHeight is the count of inner rows, so
	// its coordinate within the window is already the last valid
	// content row.
	inputY := contentHeight
	basePrompt := fmt.Sprintf("Enter choice number (1-%d): ", len(choices))

	var buf []rune
	for {
		// Clear input row within the content area.
		for x := 1; x < contentWidth+1; x++ {
			win.MovePrint(inputY, x, " ")
		}

		line := basePrompt + string(buf)
		win.MovePrint(inputY, 1, TruncateRunes(line, contentWidth))
		win.Refresh()

		ch := win.GetChar()
		if ch == 0 {
			continue
		}

		switch ch {
		case gc.Key(27): // ESC -> treat as cancellation
			return types.GptCliUIOption{}, fmt.Errorf("selection cancelled")
		case gc.KEY_ENTER, gc.KEY_RETURN:
			text := strings.TrimSpace(string(buf))
			idx, err := strconv.Atoi(text)
			if err != nil || idx < 1 || idx > len(choices) {
				// Invalid selection; show a brief error inline and
				// allow the user to try again.
				errMsg := fmt.Sprintf("Invalid selection. Enter a number 1-%d.", len(choices))
				win.MovePrint(inputY, 1, TruncateRunes(errMsg, contentWidth))
				win.Refresh()
				buf = buf[:0]
				continue
			}
			return choices[idx-1], nil
		case gc.KEY_BACKSPACE, 127, 8:
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
			}
		default:
			if ch >= '0' && ch <= '9' {
				buf = append(buf, rune(ch))
			}
		}
	}
}

// SelectBool presents a true and false option to the user via a line
// input modal and returns their selection. It mirrors the semantics of
// StdioUI.SelectBool: the user types the label of the desired option
// (case-insensitive), or presses Enter to accept the default when
// defaultOpt is non-nil.
func (n *NcursesUI) SelectBool(userPrompt string,
	trueOption, falseOption types.GptCliUIOption,
	defaultOpt *bool) (bool, error) {

	n.mu.Lock()
	defer n.mu.Unlock()

	prompt := strings.TrimRight(userPrompt, "\n")

	for {
		line, err := n.readLineModal(prompt)
		if err != nil {
			return false, err
		}

		line = strings.TrimSpace(line)
		if strings.EqualFold(line, trueOption.Label) {
			return true, nil
		}
		if strings.EqualFold(line, falseOption.Label) {
			return false, nil
		}
		if line == "" && defaultOpt != nil {
			return *defaultOpt, nil
		}

		// Invalid selection; prepend an error message and re-prompt.
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
