/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package types

type GptCliUIOption struct {
	Key   string
	Label string
}

type GptCliUIOptionDialogue interface {
	SelectOption(userPrompt string, choices []GptCliUIOption) (GptCliUIOption, error)
}

type GptCliUIInputDialogue interface {
	Get(userPrompt string) (string, error)
}
