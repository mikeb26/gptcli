/* Copyright © 2025-2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package main

import (
	"errors"
)

var (
	ErrTTYRequired                 = errors.New("A terminal is required to run gptcli")
	ErrUnknownReasoningEffort      = errors.New("Unknown reasoning effort")
	ErrEmptyMenuText               = errors.New("Empty menu text")
	ErrNilScreen                   = errors.New("Nil screen returned")
	ErrCannotEditArchivedThread    = errors.New("Cannot edit archived thread; please unarchive first")
	ErrUnsupportedVendor           = errors.New("LLM Vendor is not currently supported")
	ErrUnsupportedModel            = errors.New("LLM Model is not currently supported")
	ErrCouldNotParseLatestRelease  = errors.New("Could not parse latest release JSON")
	ErrFailedToDownloadVersion     = errors.New("Failed to download new version")
	ErrFailedToCreateTempFile      = errors.New("Failed to create temp file")
	ErrCouldNotDetermineBinaryPath = errors.New("Could not determine path to gptcli")
	ErrCouldNotReplaceBinary       = errors.New("Could not replace existing gptcli; do you need to be root?")
	ErrFailedToGetPrefsPath        = errors.New("Failed to get prefs path")
	ErrFailedToReadPrefs           = errors.New("Failed to read prefs")
	ErrFailedToMarshalPrefs        = errors.New("Failed to marshal prefs")
	ErrFailedToSavePrefs           = errors.New("Failed to save prefs")
	ErrCouldNotCreateConfigDir     = errors.New("Could not create config directory")
	ErrCouldNotOpenAPIKeyFile      = errors.New("Could not open API key file")
	ErrCouldNotWriteAPIKeyFile     = errors.New("Could not write API key file")
	ErrCouldNotCreateThreadsDir    = errors.New("Could not create thread groups directory")
	ErrCouldNotCreateLogsDir       = errors.New("Could not create logs directory")
	ErrCouldNotFindHomeDir         = errors.New("Could not find user home directory")
	ErrCouldNotLoadAPIKey          = errors.New("Could not load API key")
	ErrAPIKeyNotConfigured         = errors.New("API key not configured")
	ErrFailedToInitScreen          = errors.New("Failed to initialize screen")
	ErrFailedToPromptThreadName    = errors.New("Failed to prompt for new thread name")
	ErrFailedToCreateThread        = errors.New("Failed to create new thread")
	ErrFailedToArchiveThread       = errors.New("Failed to archive thread")
	ErrUnreachable                 = errors.New("BUG: unreachable")
	ErrCreatingHistoryFrame        = errors.New("Could not create thread history frame")
	ErrCreatingInputFrame          = errors.New("Could not create thread input frame")
)
