/* Copyright © 2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ---- git-prompt-ish implementation ----

type ps1Env struct {
	ShowDirtyState      bool
	ShowStashState      bool
	ShowUntrackedFiles  bool
	ShowUpstream        string
	ShowConflictState   string
	StateSeparator      string
	DescribeStyle       string
	CompressSparseState bool
	OmitSparseState     bool
	HideIfPwdIgnored    bool
	ShowColorHints      bool
}

// RepoStatusString returns a "git-prompt.sh-like" status string suitable for
// display in a UI title bar.
//
// It intentionally shells out to `git` for correctness and parity.
func (c *Client) RepoStatusString(ctx context.Context, dir string) (string, error) {
	env := getUserPs1Prefs()

	inside, _, err := c.runWithTimeout(ctx, buildGitArgs(dir, "rev-parse", "--is-inside-work-tree")...)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(inside) != "true" {
		return "", fmt.Errorf("%w: %v", ErrNotGitRepo, dir)
	}

	if env.HideIfPwdIgnored {
		// best-effort: hide if ignored
		_, _, err := c.runWithTimeout(ctx, buildGitArgs(dir, "check-ignore", "-q", ".")...)
		if err == nil {
			return "", fmt.Errorf("%w: %v", ErrNotGitRepo, dir)
		}
	}

	repoName := c.repoName(ctx, dir)
	branch := c.currentBranchOrDetached(ctx, env, dir)
	if branch == "" {
		return "", ErrFailedToDetermineBranch
	}

	statusOut, _, err := c.runWithTimeout(ctx, buildGitArgs(dir, "status", "--porcelain=v2", "--branch")...)
	if err != nil {
		// fallback: show at least branch
		if repoName != "" {
			return repoName + ":" + branch, nil
		}
		return branch, nil
	}
	meta, flags := parsePorcelainV2(statusOut)

	dirtyFlags := ""
	if env.ShowDirtyState {
		if flags.unstaged {
			dirtyFlags += "*"
		}
		if flags.staged {
			dirtyFlags += "+"
		}
	}

	stashFlag := ""
	if env.ShowStashState {
		_, _, err := c.runWithTimeout(ctx, buildGitArgs(dir, "rev-parse", "--verify", "--quiet", "refs/stash")...)
		if err == nil {
			stashFlag = "$"
		}
	}

	untrackedFlag := ""
	if env.ShowUntrackedFiles {
		if flags.untracked {
			untrackedFlag = "%"
		}
	}

	sparse := ""
	if !env.OmitSparseState {
		if c.trueGitConfig(ctx, dir, "core.sparseCheckout") {
			if env.CompressSparseState {
				sparse = "?"
			} else {
				sparse = "|SPARSE"
			}
		}
	}

	op := c.inProgressOperation(ctx, dir)

	conflict := ""
	if env.ShowConflictState == "yes" {
		out, _, err := c.runWithTimeout(ctx, buildGitArgs(dir, "ls-files", "--unmerged")...)
		if err == nil && strings.TrimSpace(out) != "" {
			conflict = "|CONFLICT"
		}
	}

	upstream := ""
	if env.ShowUpstream != "" {
		upstream = upstreamDivergence(env.ShowUpstream, meta)
	}

	sep := env.StateSeparator
	if sep == "" {
		sep = " "
	}

	state := dirtyFlags + stashFlag + untrackedFlag
	out := branch
	if repoName != "" {
		out = repoName + ":" + out
	}
	if state != "" {
		out = out + sep + state
	}
	out = out + sparse + op + upstream + conflict
	return strings.TrimSpace(out), nil
}

func getUserPs1Prefs() (env ps1Env) {
	present := make(map[string]bool)
	get := func(k string) string {
		v, ok := os.LookupEnv(k)
		if ok {
			present[k] = true
		}
		return v
	}
	env.ShowDirtyState = parseGitPs1Bool(get("GIT_PS1_SHOWDIRTYSTATE"))
	env.ShowStashState = parseGitPs1Bool(get("GIT_PS1_SHOWSTASHSTATE"))
	env.ShowUntrackedFiles = parseGitPs1Bool(get("GIT_PS1_SHOWUNTRACKEDFILES"))
	env.ShowUpstream = get("GIT_PS1_SHOWUPSTREAM")
	env.ShowConflictState = get("GIT_PS1_SHOWCONFLICTSTATE")

	env.StateSeparator = get("GIT_PS1_STATESEPARATOR")
	env.DescribeStyle = get("GIT_PS1_DESCRIBE_STYLE")
	env.CompressSparseState = parseGitPs1Bool(get("GIT_PS1_COMPRESSSPARSESTATE"))
	env.OmitSparseState = parseGitPs1Bool(get("GIT_PS1_OMITSPARSESTATE"))
	env.HideIfPwdIgnored = parseGitPs1Bool(get("GIT_PS1_HIDE_IF_PWD_IGNORED"))
	env.ShowColorHints = parseGitPs1Bool(get("GIT_PS1_SHOWCOLORHINTS"))

	// defaults if not explicitly set
	if !present["GIT_PS1_SHOWDIRTYSTATE"] {
		env.ShowDirtyState = true
	}
	if !present["GIT_PS1_SHOWSTASHSTATE"] {
		env.ShowStashState = true
	}
	if !present["GIT_PS1_SHOWUNTRACKEDFILES"] {
		env.ShowUntrackedFiles = true
	}
	if !present["GIT_PS1_SHOWUPSTREAM"] {
		env.ShowUpstream = "verbose"
	}
	if !present["GIT_PS1_SHOWCONFLICTSTATE"] {
		env.ShowConflictState = "yes"
	}

	if !present["GIT_PS1_STATESEPARATOR"] {
		env.StateSeparator = " "
	}

	return env
}

func buildGitArgs(dir string, args ...string) []string {
	if strings.TrimSpace(dir) == "" {
		return args
	}
	// Use -C to scope git execution to the provided directory without
	// changing the process working directory.
	return append([]string{"-C", dir}, args...)
}

func (c *Client) repoName(ctx context.Context, dir string) string {
	top, _, err := c.runWithTimeout(ctx, buildGitArgs(dir, "rev-parse", "--show-toplevel")...)
	if err != nil {
		return ""
	}
	top = strings.TrimSpace(top)
	if top == "" {
		return ""
	}
	return filepath.Base(top)
}

func (c *Client) currentBranchOrDetached(ctx context.Context, env ps1Env, dir string) string {
	out, _, err := c.runWithTimeout(ctx, buildGitArgs(dir, "symbolic-ref", "--quiet", "--short", "HEAD")...)
	if err == nil {
		b := strings.TrimSpace(out)
		if b != "" {
			return b
		}
	}

	style := env.DescribeStyle
	var args []string
	switch style {
	case "contains":
		args = []string{"describe", "--contains", "HEAD"}
	case "branch":
		args = []string{"describe", "--contains", "--all", "HEAD"}
	case "tag":
		args = []string{"describe", "--tags", "HEAD"}
	case "describe":
		args = []string{"describe", "HEAD"}
	case "default", "":
		args = []string{"describe", "--tags", "--exact-match", "HEAD"}
	default:
		args = []string{"describe", "--tags", "--exact-match", "HEAD"}
	}

	desc, _, err := c.runWithTimeout(ctx, buildGitArgs(dir, args...)...)
	if err == nil {
		d := strings.TrimSpace(desc)
		if d != "" {
			return "(" + d + ")"
		}
	}

	sha, _, err := c.runWithTimeout(ctx, buildGitArgs(dir, "rev-parse", "--short", "HEAD")...)
	if err != nil {
		return ""
	}
	sha = strings.TrimSpace(sha)
	if sha == "" {
		return ""
	}
	return "(" + sha + "...)"
}

func (c *Client) trueGitConfig(ctx context.Context, dir string, key string) bool {
	out, _, err := c.runWithTimeout(ctx, buildGitArgs(dir, "config", "--bool", key)...)
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == "true"
}

func (c *Client) inProgressOperation(ctx context.Context, dir string) string {
	gitDir, _, err := c.runWithTimeout(ctx, buildGitArgs(dir, "rev-parse", "--absolute-git-dir")...)
	if err != nil {
		return ""
	}
	g := strings.TrimSpace(gitDir)
	if g == "" {
		return ""
	}

	if isDir(g+"/rebase-merge") || isDir(g+"/rebase-apply") {
		return "|REBASE"
	}
	if fileExists(g + "/MERGE_HEAD") {
		return "|MERGING"
	}
	if c.sequencerStatus(g) != "" {
		return c.sequencerStatus(g)
	}
	if fileExists(g + "/BISECT_LOG") {
		return "|BISECTING"
	}
	return ""
}

func (c *Client) sequencerStatus(gitDir string) string {
	if fileExists(gitDir + "/CHERRY_PICK_HEAD") {
		return "|CHERRY-PICKING"
	}
	if fileExists(gitDir + "/REVERT_HEAD") {
		return "|REVERTING"
	}
	// If user committed mid-sequence, HEAD files may not exist; inspect todo.
	todoBytes, err := os.ReadFile(gitDir + "/sequencer/todo")
	if err != nil {
		return ""
	}
	todo := strings.TrimSpace(string(todoBytes))
	if todo == "" {
		return ""
	}
	// Match git-prompt.sh logic.
	for _, line := range strings.Split(todo, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "p ") || strings.HasPrefix(line, "pick ") || line == "p" || line == "pick" {
			return "|CHERRY-PICKING"
		}
		if strings.HasPrefix(line, "revert ") || line == "revert" {
			return "|REVERTING"
		}
		break
	}
	return ""
}

func upstreamDivergence(showUpstream string, meta porcelainMeta) string {
	opts := strings.Fields(showUpstream)
	verbose := false
	name := false
	for _, o := range opts {
		switch o {
		case "verbose":
			verbose = true
		case "name":
			name = true
		case "auto":
			// ignore
		default:
			// ignore legacy/git/svn
		}
	}

	if meta.upstream == "" {
		return ""
	}

	if !verbose {
		switch {
		case meta.ahead == 0 && meta.behind == 0:
			return "="
		case meta.ahead > 0 && meta.behind == 0:
			return ">"
		case meta.ahead == 0 && meta.behind > 0:
			return "<"
		default:
			return "<>"
		}
	}

	var sb strings.Builder
	sb.WriteString("|u")
	switch {
	case meta.ahead == 0 && meta.behind == 0:
		sb.WriteString("=")
	case meta.ahead > 0 && meta.behind == 0:
		sb.WriteString("+")
		sb.WriteString(strconv.Itoa(meta.ahead))
	case meta.ahead == 0 && meta.behind > 0:
		sb.WriteString("-")
		sb.WriteString(strconv.Itoa(meta.behind))
	default:
		sb.WriteString("+")
		sb.WriteString(strconv.Itoa(meta.ahead))
		sb.WriteString("-")
		sb.WriteString(strconv.Itoa(meta.behind))
	}
	if name {
		sb.WriteString(" ")
		sb.WriteString(meta.upstream)
	}
	return sb.String()
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !st.IsDir()
}

func isDir(path string) bool {
	st, err := os.Stat(path)
	if err != nil {
		return false
	}
	return st.IsDir()
}
