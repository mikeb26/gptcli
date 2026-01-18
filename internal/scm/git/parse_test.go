/* Copyright © 2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package git

import "testing"

func TestParseGitPs1Bool(t *testing.T) {
	t.Parallel()

	trueCases := []string{"1", "true", "yes", "on", "enable", "enabled", "anything", "  YES  "}
	for _, tc := range trueCases {
		if !parseGitPs1Bool(tc) {
			t.Fatalf("expected true for %q", tc)
		}
	}

	falseCases := []string{"", " ", "0", "false", "no", "off", "disable", "disabled", "  disabled  "}
	for _, tc := range falseCases {
		if parseGitPs1Bool(tc) {
			t.Fatalf("expected false for %q", tc)
		}
	}
}

func TestParsePorcelainV2_MetaAndFlags(t *testing.T) {
	t.Parallel()

	out := "" +
		"# branch.upstream origin/main\n" +
		"# branch.ab +2 -3\n" +
		"1 .M N... 100644 100644 100644 abc def file.txt\n" +
		"1 M. N... 100644 100644 100644 abc def staged.txt\n" +
		"? untracked.txt\n"

	meta, flags := parsePorcelainV2(out)
	if meta.upstream != "origin/main" {
		t.Fatalf("upstream mismatch: got %q", meta.upstream)
	}
	if meta.ahead != 2 || meta.behind != 3 {
		t.Fatalf("ahead/behind mismatch: got %+v", meta)
	}
	if !flags.unstaged {
		t.Fatalf("expected unstaged=true")
	}
	if !flags.staged {
		t.Fatalf("expected staged=true")
	}
	if !flags.untracked {
		t.Fatalf("expected untracked=true")
	}
}

func TestParsePorcelainV2_UnmergedSetsStagedAndUnstaged(t *testing.T) {
	t.Parallel()

	_, flags := parsePorcelainV2("u UU N... 100644 100644 100644 abc def conflict.txt\n")
	if !flags.staged || !flags.unstaged {
		t.Fatalf("expected staged and unstaged true for unmerged: %+v", flags)
	}
}
