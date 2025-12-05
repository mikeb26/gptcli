/* Copyright © 2023-2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package threads

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSplitBlocks(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		blocks []string
	}{
		{
			name:   "empty string",
			text:   "",
			blocks: []string{},
		},
		{
			name:   "no code blocks",
			text:   "This is a test.",
			blocks: []string{"This is a test."},
		},
		{
			name:   "single code block",
			text:   "```\ncode block\n```",
			blocks: []string{"", "```\ncode block\n"},
		},
		{
			name:   "text with code blocks",
			text:   "Some text ```\ncode block\n``` follow-up",
			blocks: []string{"Some text ", "```\ncode block\n```", " follow-up"},
		},
		{
			name:   "multiple code blocks",
			text:   "```\nfirst\n``` interlude ```\nsecond\n``` end",
			blocks: []string{"", "```\nfirst\n```", " interlude ", "```\nsecond\n```", " end"},
		},
		{
			name:   "multiline code block",
			text:   "```\nline1\nline2\nline3\n```",
			blocks: []string{"", "```\nline1\nline2\nline3\n"},
		},
		{
			name:   "code block at start and end",
			text:   "```\nstart\n``` text in between ```\nend\n```",
			blocks: []string{"", "```\nstart\n```", " text in between ", "```\nend\n"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitBlocks(tt.text)
			assert.Equal(t, tt.blocks, result)
		})
	}
}

func TestFormatHeaderTimeTodayYesterdayAndOther(t *testing.T) {
	// Fix "now" so results are deterministic.
	now := time.Date(2025, 1, 15, 10, 30, 0, 0, time.Local)

	todayTs := time.Date(2025, 1, 15, 8, 0, 0, 0, time.Local)
	yesterdayTs := time.Date(2025, 1, 14, 20, 0, 0, 0, time.Local)
	otherTs := time.Date(2024, 12, 31, 23, 59, 0, 0, time.Local)

	todayStr := formatHeaderTime(todayTs, now)
	yesterdayStr := formatHeaderTime(yesterdayTs, now)
	otherStr := formatHeaderTime(otherTs, now)

	assert.Contains(t, todayStr, "Today")
	assert.Contains(t, yesterdayStr, "Yesterday")
	assert.NotContains(t, otherStr, "Today")
	assert.NotContains(t, otherStr, "Yesterday")
}

func TestGenUniqFileNameDeterministicAndVariesWithInputs(t *testing.T) {
	base := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	name := "example-thread"
	file1 := genUniqFileName(name, base)
	file2 := genUniqFileName(name, base)

	// Deterministic for the same inputs.
	assert.Equal(t, file1, file2)

	// Changing the name should change the file name.
	otherName := "other-thread"
	file3 := genUniqFileName(otherName, base)
	assert.NotEqual(t, file1, file3)

	// Changing the timestamp should also change the file name.
	file4 := genUniqFileName(name, base.Add(time.Second))
	assert.NotEqual(t, file1, file4)
}

