/* Copyright © 2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package git

import (
	"strconv"
	"strings"
)

// parseGitPs1Bool returns a best-effort boolean interpretation of a
// GIT_PS1_* variable. For git-prompt.sh compatibility, any non-empty value is
// considered enabled except common "falsey" values.
func parseGitPs1Bool(v string) bool {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" {
		return false
	}
	switch v {
	case "0", "false", "no", "off", "disable", "disabled":
		return false
	default:
		return true
	}
}

type porcelainMeta struct {
	upstream string
	ahead    int
	behind   int
}

type porcelainFlags struct {
	staged    bool
	unstaged  bool
	untracked bool
}

func parsePorcelainV2(out string) (meta porcelainMeta, flags porcelainFlags) {
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "# branch.upstream ") {
			meta.upstream = strings.TrimSpace(strings.TrimPrefix(line, "# branch.upstream "))
			continue
		}
		if strings.HasPrefix(line, "# branch.ab ") {
			ab := strings.TrimSpace(strings.TrimPrefix(line, "# branch.ab "))
			parts := strings.Fields(ab)
			for _, p := range parts {
				if strings.HasPrefix(p, "+") {
					if n, err := strconv.Atoi(strings.TrimPrefix(p, "+")); err == nil {
						meta.ahead = n
					}
				}
				if strings.HasPrefix(p, "-") {
					if n, err := strconv.Atoi(strings.TrimPrefix(p, "-")); err == nil {
						meta.behind = n
					}
				}
			}
			continue
		}

		switch {
		case strings.HasPrefix(line, "1 ") || strings.HasPrefix(line, "2 "):
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				xy := fields[1]
				if len(xy) >= 2 {
					x := xy[0]
					y := xy[1]
					if x != '.' {
						flags.staged = true
					}
					if y != '.' {
						flags.unstaged = true
					}
				}
			}
		case strings.HasPrefix(line, "u "):
			flags.staged = true
			flags.unstaged = true
		case strings.HasPrefix(line, "? "):
			flags.untracked = true
		}
	}
	return meta, flags
}
