/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package threads

import "github.com/mikeb26/gptcli/internal/types"

// threadStateSetter adapts a *GptCliThread to the types.ThreadStateSetter
// interface so that lower layers (e.g. tools) can update thread state without
// importing the threads package.
type threadStateSetter struct {
	thread *Thread
}

var _ types.ThreadStateSetter = (*threadStateSetter)(nil)

func (s *threadStateSetter) SetThreadStateBlocked() {
	if s == nil || s.thread == nil {
		return
	}
	s.thread.SetState(GptCliThreadStateBlocked)
}

func (s *threadStateSetter) SetThreadStateRunning() {
	if s == nil || s.thread == nil {
		return
	}
	s.thread.SetState(GptCliThreadStateRunning)
}
