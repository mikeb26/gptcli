package ui

import (
	"testing"

	"github.com/mikeb26/gptcli/internal/types"
)

// These tests focus on the pure-Go and error-path behavior of NcursesUI
// that does not require an actual ncurses screen. Full interaction tests
// would require a real TTY and are better suited for integration tests.

func TestTruncateRunes_NoTruncation(t *testing.T) {
	input := "hello"
	got := TruncateRunes(input, len([]rune(input)))
	if got != input {
		t.Fatalf("expected %q, got %q", input, got)
	}
}

func TestTruncateRunes_Truncation(t *testing.T) {
	input := "こんにちは" // 5 runes
	got := TruncateRunes(input, 3)
	if got != "こんに" {
		t.Fatalf("expected %q, got %q", "こんに", got)
	}
}

func TestTruncateRunes_ZeroMax(t *testing.T) {
	input := "data"
	got := TruncateRunes(input, 0)
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestNcursesUISelectOption_NoChoicesError(t *testing.T) {
	ui := NewNcursesUI(nil)

	_, err := ui.SelectOption("Choose:", nil)
	if err == nil {
		t.Fatalf("expected error when no choices are provided")
	}
}

func TestNcursesUIGet_NoScreenError(t *testing.T) {
	ui := NewNcursesUI(nil)

	_, err := ui.Get("Prompt: ")
	if err == nil {
		t.Fatalf("expected error when NcursesUI has no screen")
	}
}

func TestNcursesUISelectBool_NoScreenError(t *testing.T) {
	ui := NewNcursesUI(nil)
	trueOpt := types.GptCliUIOption{Key: "y", Label: "Yes"}
	falseOpt := types.GptCliUIOption{Key: "n", Label: "No"}

	_, err := ui.SelectBool("Proceed?", trueOpt, falseOpt, nil)
	if err == nil {
		t.Fatalf("expected error when NcursesUI has no screen")
	}
}
