/* Copyright © 2023-2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package prompts

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPromptsWellFormed(t *testing.T) {
	assert.NotContains(t, SystemMsg, "(EXTRA")
}
