/* Copyright © 2023-2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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
