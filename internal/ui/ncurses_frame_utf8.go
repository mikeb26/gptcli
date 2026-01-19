/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package ui

import (
	"unicode/utf8"

	gc "github.com/rthornton128/goncurses"
)

// ReadUTF8KeyRune is a convenience helper mirroring the behavior of
// the thread view's UTF-8 input reconstruction. It allows frames to
// reconstruct multi-byte UTF-8 sequences from consecutive ncurses key
// codes when using InsertRune for input.
func ReadUTF8KeyRune(win *gc.Window, first gc.Key) rune {
	b0 := byte(int(first) & 0xFF)
	if b0 < 0x80 {
		return rune(b0)
	}

	var need int
	switch {
	case b0&0xE0 == 0xC0:
		need = 2
	case b0&0xF0 == 0xE0:
		need = 3
	case b0&0xF8 == 0xF0:
		need = 4
	default:
		return rune(b0)
	}

	buf := []byte{b0}
	for len(buf) < need {
		ch := win.GetChar()
		if ch == 0 {
			break
		}
		if ch < 0 || ch > 255 {
			break
		}
		b := byte(int(ch) & 0xFF)
		if b&0xC0 != 0x80 {
			break
		}
		buf = append(buf, b)
	}

	r, _ := utf8.DecodeRune(buf)
	if r == utf8.RuneError {
		return rune(b0)
	}
	return r
}
