/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package types

type UIOption struct {
	Key   string
	Label string
}

type UIOptionDialogue interface {
	SelectOption(userPrompt string, choices []UIOption) (UIOption, error)
}

type UIBoolDialogue interface {
	SelectBool(userPrompt string, trueOption, falseOption UIOption,
		defaultOpt *bool) (bool, error)
}

type UIInputDialogue interface {
	Get(userPrompt string) (string, error)
}

type UIConfirmDialogue interface {
	Confirm(userPrompt string) error
}

type UI interface {
	UIOptionDialogue
	UIBoolDialogue
	UIInputDialogue
	UIConfirmDialogue
}
