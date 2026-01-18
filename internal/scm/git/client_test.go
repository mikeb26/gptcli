/* Copyright © 2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package git

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestClient_runWithTimeout_UsesTimeoutWhenNoDeadline(t *testing.T) {
	_, cleanup := setupMockGit(t, map[string]string{
		"MOCK_GIT_SLEEP_SECS": "0.25",
		"MOCK_GIT_INSIDE":     "true",
	})
	t.Cleanup(cleanup)

	c := NewClient()
	c.Timeout = 20 * time.Millisecond
	_, _, err := c.runWithTimeout(context.Background(), "rev-parse", "--is-inside-work-tree")
	if err == nil {
		t.Fatalf("expected timeout error")
	}

	if !errors.Is(err, ErrFailedToExecuteGit) {
		t.Fatalf("expected ErrFailedToExecuteGit wrapper, got %v", err)
	}
}

func TestClient_runWithTimeout_RespectsExistingDeadline(t *testing.T) {
	_, cleanup := setupMockGit(t, map[string]string{
		"MOCK_GIT_INSIDE": "true",
	})
	t.Cleanup(cleanup)

	c := NewClient()
	c.Timeout = 1 * time.Nanosecond

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	out, _, err := c.runWithTimeout(ctx, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out == "" {
		t.Fatalf("expected output")
	}
}
