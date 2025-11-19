/* Copyright © 2023-2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func (gptCliCtx *GptCliContext) loadPrefs() error {
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
	if gptCliCtx.prefs.Vendor == "" {
		gptCliCtx.prefs.Vendor = DefaultVendor
	}

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
	fmt.Printf("Enter LLM vendor [openai, anthropic, or google]: ")
	vendor, err := gptCliCtx.input.ReadString('\n')
	if err != nil {
		return err
	}
	vendor = strings.ToLower(strings.TrimSpace(vendor))
	_, ok := DefaultModels[vendor]
	if !ok {
		return fmt.Errorf("Vendor %v is not currently supported", vendor)
	}
	gptCliCtx.prefs.Vendor = vendor

	keyPath := path.Join(configDir, fmt.Sprintf(KeyFileFmt, vendor))
	_, err = os.Stat(keyPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("Could not open %v API key file %v: %w", vendor,
			keyPath, err)
	}
	fmt.Printf("Enter your %v API key: ", vendor)
	key, err := gptCliCtx.input.ReadString('\n')
	if err != nil {
		return err
	}
	key = strings.TrimSpace(key)
	err = os.WriteFile(keyPath, []byte(key), 0600)
	if err != nil {
		return fmt.Errorf("Could not write %v API key file %v: %w", vendor,
			keyPath, err)
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

	fmt.Printf("Summarize dialogue when continuing threads? (reduces costs for less precise replies from %v) [N]: ",
		vendor)
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

	err = gptCliCtx.savePrefs()
	if err != nil {
		return err
	}

	return gptCliCtx.load(ctx)
}

func getConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("Could not find user home directory: %w", err)
	}

	return filepath.Join(homeDir, ".config", CommandName), nil
}

func getKeyPath(vendor string) (string, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, fmt.Sprintf(KeyFileFmt, vendor)), nil
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

func loadKey(vendor string) (string, error) {
	keyPath, err := getKeyPath(vendor)
	if err != nil {
		return "", fmt.Errorf("Could not load %v API key: %w", vendor, err)
	}
	data, err := os.ReadFile(keyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("Could not load %v API key: "+
				"run `%v config` to configure", vendor, CommandName)
		}
		return "", fmt.Errorf("Could not load %v API key: %w", vendor, err)
	}
	return string(data), nil
}
