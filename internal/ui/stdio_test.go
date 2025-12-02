package ui

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/mikeb26/gptcli/internal/types"
)

// helper to temporarily replace stdin and stdout
func withPipes(t *testing.T, input string, fn func()) (stdout string) {
	t.Helper()

	// Backup real stdin/stdout
	oldStdin := os.Stdin
	oldStdout := os.Stdout
	defer func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
	}()

	// Create pipe for stdin
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdin pipe: %v", err)
	}
	defer inR.Close()
	defer inW.Close()

	// Write the input to the write end and close it so reads hit EOF when done
	go func() {
		defer inW.Close()
		io.WriteString(inW, input)
	}()

	// Create pipe for stdout
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}
	defer outR.Close()
	defer outW.Close()

	os.Stdin = inR
	os.Stdout = outW

	// Capture output concurrently
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, outR)
		close(done)
	}()

	fn()

	// Close write end so reader terminates
	outW.Close()
	<-done

	return buf.String()
}

func TestStdioUISelectOptionValid(t *testing.T) {
	ui := NewStdioUI()

	// User selects option 2 followed by newline
	stdout := withPipes(t, "2\n", func() {
		opt, err := ui.SelectOption("Choose one:", []types.GptCliUIOption{{Key: "a", Label: "Option A"}, {Key: "b", Label: "Option B"}})
		if err != nil {
			t.Fatalf("SelectOption returned error: %v", err)
		}
		if opt.Key != "b" {
			t.Fatalf("expected option with key 'b', got %q", opt.Key)
		}
	})

	if !strings.Contains(stdout, "Choose one:") {
		t.Errorf("expected prompt to contain 'Choose one:', got %q", stdout)
	}
	if !strings.Contains(stdout, "1) Option A") || !strings.Contains(stdout, "2) Option B") {
		t.Errorf("expected options list in stdout, got %q", stdout)
	}
}

func TestStdioUISelectOptionInvalidThenValid(t *testing.T) {
	ui := NewStdioUI()

	// First input invalid ("abc"), then valid ("1")
	stdout := withPipes(t, "abc\n1\n", func() {
		opt, err := ui.SelectOption("Choose:", []types.GptCliUIOption{{Key: "x", Label: "X"}})
		if err != nil {
			t.Fatalf("SelectOption returned error: %v", err)
		}
		if opt.Key != "x" {
			t.Fatalf("expected option with key 'x', got %q", opt.Key)
		}
	})

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
	ui := NewStdioUI()

	stdout := withPipes(t, "hello world\n", func() {
		val, err := ui.Get("Enter value: ")
		if err != nil {
			t.Fatalf("Get returned error: %v", err)
		}
		if val != "hello world" {
			t.Fatalf("expected 'hello world', got %q", val)
		}
	})

	if !strings.Contains(stdout, "Enter value:") {
		t.Errorf("expected stdout to contain prompt, got %q", stdout)
	}
}

func TestStdioUIGetTrimsCRLF(t *testing.T) {
	ui := NewStdioUI()

	_ = withPipes(t, "windows line\r\n", func() {
		val, err := ui.Get("Enter: ")
		if err != nil {
			t.Fatalf("Get returned error: %v", err)
		}
		if val != "windows line" {
			t.Fatalf("expected 'windows line', got %q", val)
		}
	})
}
