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
	"syscall"
	"time"

	"github.com/mikeb26/gptcli/internal/types"
)

//go:embed version.txt
var versionText string

const DevVersionText = "v0.devbuild"

func upgradeIfNeeded(ctx context.Context, gptCliCtx *CliContext) error {
	if versionText == DevVersionText {
		return nil
	}
	latestVer, err := getLatestVersion()
	if err != nil {
		return err
	}
	if latestVer == versionText {
		return nil
	}

	prompt := fmt.Sprintf("A new version of gptcli is available (%v). Upgrade?",
		latestVer)
	trueOpt := types.UIOption{Key: "y", Label: "y"}
	falseOpt := types.UIOption{Key: "n", Label: "n"}
	defaultYes := true
	shouldUpgrade, err := gptCliCtx.ui.SelectBool(prompt, trueOpt, falseOpt,
		&defaultYes)
	if err != nil {
		return err
	}
	if !shouldUpgrade {
		return nil
	}

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
		return "", fmt.Errorf("%w: %v", ErrCouldNotParseLatestRelease, LatestReleaseUrl)
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
		return fmt.Errorf("%w %v: %w", ErrFailedToDownloadVersion, versionText, err)

	}

	tmpFile, err := os.CreateTemp("", "gptcli-*")
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToCreateTempFile, err)
	}
	binaryContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%w %v: %w", ErrFailedToDownloadVersion, versionText, err)
	}
	_, err = tmpFile.Write(binaryContent)
	if err != nil {
		return fmt.Errorf("%w %v: %w", ErrFailedToDownloadVersion, versionText, err)
	}
	err = tmpFile.Chmod(0755)
	if err != nil {
		return fmt.Errorf("%w %v: %w", ErrFailedToDownloadVersion, versionText, err)
	}
	err = tmpFile.Close()
	if err != nil {
		return fmt.Errorf("%w %v: %w", ErrFailedToDownloadVersion, versionText, err)
	}
	myBinaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCouldNotDetermineBinaryPath, err)
	}
	myBinaryPath, err = filepath.EvalSymlinks(myBinaryPath)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCouldNotDetermineBinaryPath, err)
	}

	myBinaryPathBak := myBinaryPath + ".bak"
	err = os.Rename(myBinaryPath, myBinaryPathBak)
	if err != nil {
		return fmt.Errorf("%w %v: %w", ErrCouldNotReplaceBinary, myBinaryPath, err)
	}
	err = os.Rename(tmpFile.Name(), myBinaryPath)
	if errors.Is(err, syscall.EXDEV) {
		// invalid cross device link occurs when rename() is attempted aross
		// different filesystems; copy instead
		err = os.WriteFile(myBinaryPath, binaryContent, 0755)
		_ = os.Remove(tmpFile.Name())
	}
	if err != nil {
		err := fmt.Errorf("%w %v: %w", ErrCouldNotReplaceBinary, myBinaryPath, err)
		_ = os.Rename(myBinaryPathBak, myBinaryPath)
		return err
	}
	_ = os.Remove(myBinaryPathBak)

	return nil
}
