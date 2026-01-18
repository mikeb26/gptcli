/* Copyright © 2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package git

import (
	_ "embed"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

//go:embed mockgit.sh
var mockGitText string

// setupMockGit puts a small "git" shim at the front of PATH.
//
// The shim is driven entirely by environment variables so tests can be
// deterministic without depending on a real git binary or a real repository.
func setupMockGit(t *testing.T, env map[string]string) (logPath string, cleanup func()) {
	t.Helper()

	if runtime.GOOS == "windows" {
		// These tests rely on a POSIX shell shim. If/when Windows support is
		// needed, replace this with a small helper .exe.
		t.Skip("mock git shim currently requires a POSIX shell")
	}

	rootDir, err := os.MkdirTemp("", "mockgit-")
	if err != nil {
		t.Fatalf("make temp dir: %v", err)
	}
	cleanup = func() {
		_ = os.RemoveAll(rootDir)
	}

	binDir := filepath.Join(rootDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		cleanup()
		t.Fatalf("mkdir bin dir: %v", err)
	}

	logPath = filepath.Join(rootDir, "git.log")
	gitPath := filepath.Join(binDir, "git")

	if err := os.WriteFile(gitPath, []byte(mockGitText), 0o755); err != nil {
		cleanup()
		t.Fatalf("write mock git: %v", err)
	}

	// Ensure our mock git is found first.
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath)
	t.Setenv("MOCK_GIT_LOG", logPath)

	for k, v := range env {
		t.Setenv(k, v)
	}

	return logPath, cleanup
}

func readMockGitLog(t *testing.T, logPath string) []string {
	t.Helper()
	bs, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read mock git log: %v", err)
	}
	// Keep it simple: split on \n and drop trailing empty.
	lines := make([]string, 0)
	start := 0
	for i := 0; i < len(bs); i++ {
		if bs[i] == '\n' {
			line := string(bs[start:i])
			if line != "" {
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	if start < len(bs) {
		line := string(bs[start:])
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
