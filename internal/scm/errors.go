/* Copyright © 2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package scm

import (
	"errors"
)

var (
	ErrUntrackedFiles  = errors.New("untracked files are present")
	ErrNothingToCommit = errors.New("there is nothing to commit")
)
