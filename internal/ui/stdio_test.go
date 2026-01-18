/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mikeb26/gptcli/internal/types"
)

func TestStdioUISelectOptionValid(t *testing.T) {
	var out bytes.Buffer
	ui := NewStdioUI().WithReader(strings.NewReader("2\n")).WithWriter(&out)

	opt, err := ui.SelectOption("Choose one:", []types.UIOption{{Key: "a", Label: "Option A"}, {Key: "b", Label: "Option B"}})
	if err != nil {
		t.Fatalf("SelectOption returned error: %v", err)
	}
	if opt.Key != "b" {
		t.Fatalf("expected option with key 'b', got %q", opt.Key)
	}

	stdout := out.String()
	if !strings.Contains(stdout, "Choose one:") {
		t.Errorf("expected prompt to contain 'Choose one:', got %q", stdout)
	}
	if !strings.Contains(stdout, "1) Option A") || !strings.Contains(stdout, "2) Option B") {
		t.Errorf("expected options list in stdout, got %q", stdout)
	}
}

func TestStdioUISelectOptionInvalidThenValid(t *testing.T) {
	var out bytes.Buffer
	ui := NewStdioUI().WithReader(strings.NewReader("abc\n1\n")).WithWriter(&out)

	opt, err := ui.SelectOption("Choose:", []types.UIOption{{Key: "x", Label: "X"}})
	if err != nil {
		t.Fatalf("SelectOption returned error: %v", err)
	}
	if opt.Key != "x" {
		t.Fatalf("expected option with key 'x', got %q", opt.Key)
	}

	stdout := out.String()
	if !strings.Contains(stdout, "Invalid selection") {
		t.Errorf("expected stdout to contain invalid selection message, got %q", stdout)
	}
}

func TestStdioUISelectOptionNoChoices(t *testing.T) {
	ui := NewStdioUI()

	_, err := ui.SelectOption("Choose:", nil)
	if err == nil {
		t.Fatalf("expected error when no choices provided")
	}
}

func TestStdioUIGetTrimsNewline(t *testing.T) {
	var out bytes.Buffer
	ui := NewStdioUI().WithReader(strings.NewReader("hello world\n")).WithWriter(&out)

	val, err := ui.Get("Enter value: ")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if val != "hello world" {
		t.Fatalf("expected 'hello world', got %q", val)
	}

	stdout := out.String()
	if !strings.Contains(stdout, "Enter value:") {
		t.Errorf("expected stdout to contain prompt, got %q", stdout)
	}
}

func TestStdioUIGetTrimsCRLF(t *testing.T) {
	var out bytes.Buffer
	ui := NewStdioUI().WithReader(strings.NewReader("windows line\r\n")).WithWriter(&out)

	val, err := ui.Get("Enter: ")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if val != "windows line" {
		t.Fatalf("expected 'windows line', got %q", val)
	}
}

func TestStdioUISelectBool(t *testing.T) {
	var out bytes.Buffer
	// User first enters invalid text, then selects true option, then just hits
	// enter to use the default.
	ui := NewStdioUI().WithReader(strings.NewReader("maybe\nYes\n\n")).WithWriter(&out)

	trueOpt := types.UIOption{Key: "y", Label: "Yes"}
	falseOpt := types.UIOption{Key: "n", Label: "No"}
	defaultTrue := true

	// First call: invalid -> true (no default used)
	val, err := ui.SelectBool("Proceed? ", trueOpt, falseOpt, nil)
	if err != nil {
		t.Fatalf("SelectBool returned error: %v", err)
	}
	if !val {
		t.Fatalf("expected true after selecting 'Yes', got false")
	}

	// Second call: empty input -> default true
	val, err = ui.SelectBool("Proceed with default? ", trueOpt, falseOpt, &defaultTrue)
	if err != nil {
		t.Fatalf("SelectBool (with default) returned error: %v", err)
	}
	if !val {
		t.Fatalf("expected true from default selection, got false")
	}

	stdout := out.String()
	if !strings.Contains(stdout, "Proceed? ") {
		t.Errorf("expected first prompt in stdout, got %q", stdout)
	}
	if !strings.Contains(stdout, "Invalid selection.") {
		t.Errorf("expected invalid selection message in stdout, got %q", stdout)
	}
}

func TestStdioUIConfirm(t *testing.T) {
	var out bytes.Buffer
	ui := NewStdioUI().WithReader(strings.NewReader("\n")).WithWriter(&out)

	err := ui.Confirm("Press enter")
	if err != nil {
		t.Fatalf("Confirm returned error: %v", err)
	}

	stdout := out.String()
	if !strings.Contains(stdout, "Press enter") {
		t.Errorf("expected stdout to contain prompt, got %q", stdout)
	}
	if !strings.Contains(stdout, "OK") {
		t.Errorf("expected stdout to contain OK, got %q", stdout)
	}
}
