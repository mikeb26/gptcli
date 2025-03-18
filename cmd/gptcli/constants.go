/* Copyright © 2023-2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package main

const DefaultVendor = "openai"

var DefaultModels = map[string]string{
	"anthropic": "claude-3-7-sonnet-20250219",
	"openai":    "o3-mini",
}

const MaxDepth = 3
