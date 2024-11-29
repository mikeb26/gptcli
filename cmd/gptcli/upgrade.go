/* Copyright © 2023-2024 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

//go:embed version.txt
var versionText string

const DevVersionText = "v0.devbuild"

func versionMain(ctx context.Context, gptCliCtx *GptCliContext, args []string) error {
	fmt.Printf("gptcli-%v\n", versionText)

	return nil
}

func upgradeMain(ctx context.Context, gptCliCtx *GptCliContext, args []string) error {
	if versionText == DevVersionText {
		fmt.Fprintf(os.Stderr, "Skipping gptcli upgrade on development version\n")
		return nil
	}
	latestVer, err := getLatestVersion()
	if err != nil {
		return err
	}
	if latestVer == versionText {
		fmt.Printf("gptcli %v is already the latest version\n",
			versionText)
		return nil
	}

	fmt.Printf("A new version of gptcli is available (%v). Upgrade? (Y/N) [Y]: ",
		latestVer)
	shouldUpgrade, err := gptCliCtx.input.ReadString('\n')
	if err != nil {
		return err
	}

	shouldUpgrade = strings.ToUpper(strings.TrimSpace(shouldUpgrade))
	if len(shouldUpgrade) == 0 {
		shouldUpgrade = "Y"
	}
	if shouldUpgrade[0] != 'Y' {
		return nil
	}

	fmt.Printf("Upgrading gptcli from %v to %v...\n", versionText,
		latestVer)

	err = upgradeViaGithub(latestVer)
	if err != nil {
		return err
	}

	return io.EOF
}

func getLatestVersion() (string, error) {
	const LatestReleaseUrl = "https://api.github.com/repos/mikeb26/gptcli/releases/latest"

	client := http.Client{
		Timeout: time.Second * 30,
	}

	resp, err := client.Get(LatestReleaseUrl)
	if err != nil {
		return "", err
	}

	releaseJsonDoc, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var releaseDoc map[string]any
	err = json.Unmarshal(releaseJsonDoc, &releaseDoc)
	if err != nil {
		return "", err
	}

	latestRelease, ok := releaseDoc["tag_name"].(string)
	if !ok {
		return "", fmt.Errorf("Could not parse %v", LatestReleaseUrl)
	}

	return latestRelease, nil
}

func upgradeViaGithub(latestVer string) error {
	const LatestDownloadFmt = "https://github.com/mikeb26/gptcli/releases/download/%v/gptcli"

	client := http.Client{
		Timeout: time.Second * 30,
	}

	resp, err := client.Get(fmt.Sprintf(LatestDownloadFmt, latestVer))
	if err != nil {
		return fmt.Errorf("Failed to download version %v: %w", versionText, err)

	}

	tmpFile, err := os.CreateTemp("", "gptcli-*")
	if err != nil {
		return fmt.Errorf("Failed to create temp file: %w", err)
	}
	binaryContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Failed to download version %v: %w", versionText, err)
	}
	_, err = tmpFile.Write(binaryContent)
	if err != nil {
		return fmt.Errorf("Failed to download version %v: %w", versionText, err)
	}
	err = tmpFile.Chmod(0755)
	if err != nil {
		return fmt.Errorf("Failed to download version %v: %w", versionText, err)
	}
	err = tmpFile.Close()
	if err != nil {
		return fmt.Errorf("Failed to download version %v: %w", versionText, err)
	}
	myBinaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("Could not determine path to gptcli: %w", err)
	}
	myBinaryPath, err = filepath.EvalSymlinks(myBinaryPath)
	if err != nil {
		return fmt.Errorf("Could not determine path to gptcli: %w", err)
	}

	myBinaryPathBak := myBinaryPath + ".bak"
	err = os.Rename(myBinaryPath, myBinaryPathBak)
	if err != nil {
		return fmt.Errorf("Could not replace existing %v; do you need to be root?: %w",
			myBinaryPath, err)
	}
	err = os.Rename(tmpFile.Name(), myBinaryPath)
	if errors.Is(err, syscall.EXDEV) {
		// invalid cross device link occurs when rename() is attempted aross
		// different filesystems; copy instead
		err = os.WriteFile(myBinaryPath, binaryContent, 0755)
		_ = os.Remove(tmpFile.Name())
	}
	if err != nil {
		err := fmt.Errorf("Could not replace existing %v; do you need to be root?: %w",
			myBinaryPath, err)
		_ = os.Rename(myBinaryPathBak, myBinaryPath)
		return err
	}
	_ = os.Remove(myBinaryPathBak)

	fmt.Printf("Upgrade %v to %v complete\n", myBinaryPath, latestVer)

	return nil
}

func checkAndPrintUpgradeWarning() bool {
	if versionText == DevVersionText {
		return false
	}
	latestVer, err := getLatestVersion()
	if err != nil {
		return false
	}
	if latestVer == versionText {
		return false
	}

	fmt.Fprintf(os.Stderr, "*WARN*: A new version of gptcli is available (%v). Please upgrade via 'upgrade'.\n\n",
		latestVer)

	return true
}
