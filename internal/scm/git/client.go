/* Copyright © 2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// Client is a small abstraction over calling the git executable.
//
// gptcli assumes git is installed and available, and uses this client
// as the backend for git-related functionality.
//
// Keeping all git process invocation behind this type makes it easier
// to:
//   - add timeouts/cancellation consistently (CommandContext)
//   - inject a fake client in tests (later, if needed)
//   - centralize logging/telemetry later
//   - evolve to higher level git helpers without exec sprawl
//
// NOTE: this is intentionally minimal and should grow as the repository
// adds more git features (commit/merge/etc).
type Client struct {
	// Timeout is applied when ctx has no deadline.
	Timeout time.Duration
}

func NewClient() *Client {
	return &Client{Timeout: 750 * time.Millisecond}
}

func (c *Client) runWithTimeout(ctx context.Context, args ...string) (string, string, error) {
	if _, ok := ctx.Deadline(); !ok && c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}

	return c.run(ctx, args...)
}

func (c *Client) run(ctx context.Context, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	var out bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if err != nil {
		return out.String(), errBuf.String(),
			fmt.Errorf("%w: %w", ErrFailedToExecuteGit, err)
	}
	return out.String(), errBuf.String(), nil
}
