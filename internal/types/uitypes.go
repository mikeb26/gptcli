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

type GptCliUIBoolDialogue interface {
	SelectBool(userPrompt string, trueOption, falseOption GptCliUIOption,
		defaultOpt *bool) (bool, error)
}

type GptCliUIInputDialogue interface {
	Get(userPrompt string) (string, error)
}

type GptCliUI interface {
	GptCliUIOptionDialogue
	GptCliUIBoolDialogue
	GptCliUIInputDialogue
}
