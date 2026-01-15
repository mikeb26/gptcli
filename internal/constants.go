/* Copyright © 2023-2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package internal

var SupportedModels = map[string][]string{
	"google": []string{"gemini-3-pro-preview", "gemini-3-flash-preview"},
	"anthropic": []string{"claude-sonnet-4-5-20250929",
		"claude-opus-4-5-20251101", "claude-haiku-4-5-20251001"},
	"openai": []string{"gpt-5.2", "gpt-5-mini", "gpt-5.2-pro"},
}

const DefaultVendor = "openai"

// key is the index into SupportedModels
var DefaultModels = map[string]int{
	"google":    0,
	"anthropic": 0,
	"openai":    0,
}

const MaxDepth = 3
