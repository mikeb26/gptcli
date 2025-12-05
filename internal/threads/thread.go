/* Copyright © 2023-2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package threads

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mikeb26/gptcli/internal/types"
)

const (
	ThreadNoExistErrFmt = "Thread %v does not exist. To list threads try 'ls'.\n"
	RowFmt              = "│ %8v │ %18v │ %18v │ %18v │ %-18v\n"
	RowSpacer           = "──────────────────────────────────────────────────────────────────────────────────────────────\n"
)

type GptCliThread struct {
	Name       string                 `json:"name"`
	CreateTime time.Time              `json:"ctime"`
	AccessTime time.Time              `json:"atime"`
	ModTime    time.Time              `json:"mtime"`
	Dialogue   []*types.GptCliMessage `json:"dialogue"`

	fileName string
}

func (thread *GptCliThread) save(dir string) error {
	threadFileContent, err := json.Marshal(thread)
	if err != nil {
		return fmt.Errorf("Failed to save thread %v: %w", thread.Name, err)
	}

	filePath := filepath.Join(dir, thread.fileName)
	err = os.WriteFile(filePath, threadFileContent, 0600)
	if err != nil {
		return fmt.Errorf("Failed to save thread %v(%v): %w", thread.Name,
			filePath, err)
	}

	return nil
}

func (thread *GptCliThread) remove(dir string) error {
	filePath := filepath.Join(dir, thread.fileName)
	err := os.Remove(filePath)
	if err != nil {
		return fmt.Errorf("Failed to delete thread %v(%v): %w", thread.Name,
			filePath, err)
	}

	return nil
}

func (t *GptCliThread) HeaderString(threadNum string) string {
	now := time.Now()

	cTime := formatHeaderTime(t.CreateTime, now)
	aTime := formatHeaderTime(t.AccessTime, now)
	mTime := formatHeaderTime(t.ModTime, now)

	return fmt.Sprintf(RowFmt, threadNum, aTime, mTime, cTime, t.Name)
}
