/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package main

import (
	"fmt"
)

var (
	ErrNoThreadsExist = fmt.Errorf("You haven't created any threads yet. To create a thread use the 'new' command")
)
