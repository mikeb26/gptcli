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
	"sort"
	"strings"

	"github.com/mikeb26/gptcli/internal"
	"github.com/mikeb26/gptcli/internal/types"
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
		gptCliCtx.prefs.Vendor = internal.DefaultVendor
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

func configMain(ctx context.Context, gptCliCtx *GptCliContext) error {
	configDir, err := getConfigDir()
	if err != nil {
		return err
	}
	err = os.MkdirAll(configDir, 0700)
	if err != nil {
		return fmt.Errorf("Could not create config directory %v: %w",
			configDir, err)
	}

	// Build vendor options from internal.DefaultModels so the list stays in sync
	vendorKeys := make([]string, 0, len(internal.DefaultModels))
	for v := range internal.DefaultModels {
		vendorKeys = append(vendorKeys, v)
	}
	sort.Strings(vendorKeys)
	choices := make([]types.GptCliUIOption, 0, len(vendorKeys))
	for _, v := range vendorKeys {
		choices = append(choices, types.GptCliUIOption{Key: v, Label: v})
	}

	selection, err := gptCliCtx.ui.SelectOption("Choose an LLM vendor:", choices)
	if err != nil {
		return err
	}
	vendor := strings.ToLower(strings.TrimSpace(selection.Key))
	if _, ok := internal.DefaultModels[vendor]; !ok {
		return fmt.Errorf("Vendor %v is not currently supported", vendor)
	}
	gptCliCtx.prefs.Vendor = vendor

	keyPath := path.Join(configDir, fmt.Sprintf(KeyFileFmt, vendor))
	_, err = os.Stat(keyPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("Could not open %v API key file %v: %w", vendor,
			keyPath, err)
	}
	keyPrompt := fmt.Sprintf("Enter your %v API key: ", vendor)
	key, err := gptCliCtx.ui.Get(keyPrompt)
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

	summarizePrompt := fmt.Sprintf(
		"Summarize dialogue when continuing threads? (reduces costs for less precise replies from %v) (y/n) [n]: ",
		vendor,
	)
	defaultSummarize := false
	trueOpt := types.GptCliUIOption{Key: "y", Label: "y"}
	falseOpt := types.GptCliUIOption{Key: "n", Label: "n"}

	summarize, err := gptCliCtx.ui.SelectBool(summarizePrompt, trueOpt, falseOpt, &defaultSummarize)
	if err != nil {
		return err
	}

	gptCliCtx.prefs.SummarizePrior = summarize
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
