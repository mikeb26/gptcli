/* Copyright © 2023-2024 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/sashabaranov/go-openai"
)

func (gptCliCtx *GptCliContext) loadPrefs() error {
	if gptCliCtx.needConfig {
		return nil
	}

	filePath, err := getPrefsPath()
	if err != nil {
		return fmt.Errorf("Failed to get prefs path: %w", err)
	}
	prefsFileContent, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("Failed to read prefs: %w", err)
	}
	err = json.Unmarshal(prefsFileContent, &gptCliCtx.prefs)
	if err != nil {
		return err
	}
	gptCliCtx.curSummaryToggle = gptCliCtx.prefs.SummarizePrior

	return nil
}

func (gptCliCtx *GptCliContext) savePrefs() error {
	prefsFileContent, err := json.Marshal(gptCliCtx.prefs)
	if err != nil {
		return fmt.Errorf("Failed to marshal prefs: %w", err)
	}

	filePath, err := getPrefsPath()
	if err != nil {
		return fmt.Errorf("Failed to get prefs path: %w", err)
	}
	err = os.WriteFile(filePath, prefsFileContent, 0600)
	if err != nil {
		return fmt.Errorf("Failed to save prefs: %w", err)
	}

	return nil
}

func checkAndUpgradeConfig() {
	// versions v0.3.5 and earlier don't have the archive dir
	archiveDir, err := getArchiveDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "*WARN*: Unable to add archive directory: %v\n",
			err)
		return
	}
	err = os.MkdirAll(archiveDir, 0700)
	if err != nil {
		fmt.Fprintf(os.Stderr, "*WARN*: Unable to add archive directory %v: %v",
			archiveDir, err)
		return
	}
}

func configMain(ctx context.Context, gptCliCtx *GptCliContext, args []string) error {
	configDir, err := getConfigDir()
	if err != nil {
		return err
	}
	err = os.MkdirAll(configDir, 0700)
	if err != nil {
		return fmt.Errorf("Could not create config directory %v: %w",
			configDir, err)
	}
	keyPath := path.Join(configDir, KeyFile)
	_, err = os.Stat(keyPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("Could not open OpenAI API key file %v: %w", keyPath, err)
	}
	fmt.Printf("Enter your OpenAI API key: ")
	key, err := gptCliCtx.input.ReadString('\n')
	if err != nil {
		return err
	}
	key = strings.TrimSpace(key)
	err = os.WriteFile(keyPath, []byte(key), 0600)
	if err != nil {
		return fmt.Errorf("Could not write OpenAI API key file %v: %w", keyPath, err)
	}
	threadsPath := path.Join(configDir, ThreadsDir)
	err = os.MkdirAll(threadsPath, 0700)
	if err != nil {
		return fmt.Errorf("Could not create threads directory %v: %w",
			threadsPath, err)
	}
	archivePath := path.Join(configDir, ArchiveDir)
	err = os.MkdirAll(archivePath, 0700)
	if err != nil {
		return fmt.Errorf("Could not create archive directory %v: %w",
			archivePath, err)
	}

	gptCliCtx.client = openai.NewClient(key)
	gptCliCtx.needConfig = false

	fmt.Printf("Summarize dialogue when continuing threads? (reduces costs for less precise replies from OpenAI) [N]: ")
	shouldSummarize, err := gptCliCtx.input.ReadString('\n')
	if err != nil {
		return err
	}

	shouldSummarize = strings.ToUpper(strings.TrimSpace(shouldSummarize))
	if len(shouldSummarize) == 0 {
		shouldSummarize = "N"
	}
	gptCliCtx.prefs.SummarizePrior = (shouldSummarize[0] == 'Y')
	gptCliCtx.curSummaryToggle = gptCliCtx.prefs.SummarizePrior

	return gptCliCtx.savePrefs()
}

func getConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("Could not find user home directory: %w", err)
	}

	return filepath.Join(homeDir, ".config", CommandName), nil
}

func getKeyPath() (string, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, KeyFile), nil
}

func getPrefsPath() (string, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, PrefsFile), nil
}

func getThreadsDir() (string, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, ThreadsDir), nil
}

func getArchiveDir() (string, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, ArchiveDir), nil
}

func loadKey() (string, error) {
	keyPath, err := getKeyPath()
	if err != nil {
		return "", fmt.Errorf("Could not load OpenAI API key: %w", err)
	}
	data, err := os.ReadFile(keyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("Could not load OpenAI API key: "+
				"run `%v config` to configure", CommandName)
		}
		return "", fmt.Errorf("Could not load OpenAI API key: %w", err)
	}
	return string(data), nil
}
