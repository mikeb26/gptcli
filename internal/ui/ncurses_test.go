/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package ui

import (
	"testing"
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
