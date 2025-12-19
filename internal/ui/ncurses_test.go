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

func TestWrapRunesWithContinuation_Empty(t *testing.T) {
	segments, wrapped := WrapRunesWithContinuation([]rune{}, 5)
	if len(segments) != 1 || len(wrapped) != 1 {
		t.Fatalf("expected one empty segment, got %d/%d", len(segments), len(wrapped))
	}
	if len(segments[0]) != 0 || wrapped[0] {
		t.Fatalf("expected empty/unwrapped, got %q wrapped=%v", string(segments[0]), wrapped[0])
	}
}

func TestWrapRunesWithContinuation_NoWrap(t *testing.T) {
	segments, wrapped := WrapRunesWithContinuation([]rune("hello"), 10)
	if len(segments) != 1 || len(wrapped) != 1 {
		t.Fatalf("expected one segment, got %d/%d", len(segments), len(wrapped))
	}
	if string(segments[0]) != "hello" || wrapped[0] {
		t.Fatalf("expected %q/unwrapped, got %q wrapped=%v", "hello", string(segments[0]), wrapped[0])
	}
}

func TestWrapRunesWithContinuation_WrapWidthWithMarker(t *testing.T) {
	segments, wrapped := WrapRunesWithContinuation([]rune("abcdefgh"), 4)
	// width=4 => continuation segments length 3
	if len(segments) != 3 {
		t.Fatalf("expected 3 segments, got %d", len(segments))
	}
	if string(segments[0]) != "abc" || !wrapped[0] {
		t.Fatalf("expected first segment abc wrapped, got %q wrapped=%v", string(segments[0]), wrapped[0])
	}
	if string(segments[1]) != "def" || !wrapped[1] {
		t.Fatalf("expected second segment def wrapped, got %q wrapped=%v", string(segments[1]), wrapped[1])
	}
	if string(segments[2]) != "gh" || wrapped[2] {
		t.Fatalf("expected last segment gh unwrapped, got %q wrapped=%v", string(segments[2]), wrapped[2])
	}
}
