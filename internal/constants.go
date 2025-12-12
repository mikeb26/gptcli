/* Copyright © 2023-2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package internal

const DefaultVendor = "openai"

var DefaultModels = map[string]string{
	"google":    "gemini-3-pro-preview",
	"anthropic": "claude-sonnet-4-5-20250929",
	"openai":    "gpt-5.2",
}

const MaxDepth = 3
