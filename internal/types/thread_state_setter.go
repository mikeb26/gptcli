/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package types

import "context"

// ThreadStateSetter is an optional interface that can be attached to a context
// by higher layers (e.g. threads) so that lower layers (e.g. tool approval UI)
// can signal that the active thread is blocked/running without importing the
// threads package (avoids import cycles).
type ThreadStateSetter interface {
	SetThreadStateBlocked()
	SetThreadStateRunning()
}

type threadStateSetterKey struct{}

// WithThreadStateSetter returns a context with a ThreadStateSetter attached.
func WithThreadStateSetter(ctx context.Context, setter ThreadStateSetter) context.Context {
	return context.WithValue(ctx, threadStateSetterKey{}, setter)
}

// GetThreadStateSetter retrieves a ThreadStateSetter from a context, if any.
func GetThreadStateSetter(ctx context.Context) (ThreadStateSetter, bool) {
	if v := ctx.Value(threadStateSetterKey{}); v != nil {
		if s, ok := v.(ThreadStateSetter); ok && s != nil {
			return s, true
		}
	}
	return nil, false
}
