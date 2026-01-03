/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package threads

import (
	"context"
	"fmt"
	"sync"

	"github.com/mikeb26/gptcli/internal/am"
)

// AsyncApprover is an Approver implementation that forwards approval requests
// over a channel so they can be handled by a single approver-owning goroutine.
//
// Usage:
//  1. Create: a := am.NewAsyncApprover(realApprover, 0)
//  2. Pass anywhere an am.Approver is needed.
//  3. In the goroutine that owns realApprover calls, read from a.Requests and
//     execute them using ServeAsyncApprovalRequest(realApprover, req).
//
// Note: Close marks the proxy closed but does NOT close the Requests channel.
// Callers might still attempt to send; lifecycle management belongs to the
// goroutine servicing Requests.
//
// AsyncApprover wraps an underlying Approver primarily to allow the serving
// goroutine to call a.ServeRequest(req) instead of passing the underlying
// explicitly.
//
// AskApproval blocks until a decision is returned on the per-request reply
// channel or the provided context is canceled.
type AsyncApprover struct {
	// Requests is the channel on which approval requests are delivered.
	Requests chan AsyncApprovalRequest

	underlying am.Approver

	mu     sync.RWMutex
	closed bool
}

func NewAsyncApprover(underlyingIn am.Approver) *AsyncApprover {
	return &AsyncApprover{
		Requests:   make(chan AsyncApprovalRequest, 1),
		underlying: underlyingIn,
	}
}

// Close marks the proxy closed. After Close, any new AskApproval call returns
// an error.
func (a *AsyncApprover) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closed = true
}

func (a *AsyncApprover) isClosed() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.closed
}

func (a *AsyncApprover) AskApproval(ctx context.Context, req am.ApprovalRequest) (am.ApprovalDecision, error) {
	if a.isClosed() {
		return am.ApprovalDecision{}, fmt.Errorf("async approver closed")
	}

	replyCh := make(chan AsyncApprovalResponse, 1)
	wrapped := AsyncApprovalRequest{
		Ctx:     ctx,
		Request: req,
		ReplyCh: replyCh,
	}

	// If the caller attached a thread-state setter to the context, mark the
	// thread blocked while we prompt for user input.
	a.setThreadState(ctx, ThreadStateBlocked)
	defer a.setThreadState(ctx, ThreadStateRunning)

	// send the approval request
	select {
	case <-ctx.Done():
		return am.ApprovalDecision{}, ctx.Err()
	case a.Requests <- wrapped:
	}

	// wait for the approval decision
	select {
	case <-ctx.Done():
		return am.ApprovalDecision{}, ctx.Err()
	case resp := <-replyCh:
		return resp.Decision, resp.Err
	}
}

// AsyncApprovalRequest is a marshaled approval interaction to be executed by
// an approver-owning goroutine.
//
// ThreadID is optional metadata for consumers that want to route/queue
// requests by thread.
//
// Ctx is the context supplied to AskApproval; it may be nil.
type AsyncApprovalRequest struct {
	Ctx      context.Context
	ThreadID string
	Request  am.ApprovalRequest
	ReplyCh  chan AsyncApprovalResponse
}

type AsyncApprovalResponse struct {
	Decision am.ApprovalDecision
	Err      error
}

// ServeRequest executes req against the AsyncApprover's underlying approver and
// sends the result back on req.ReplyCh.
func (a *AsyncApprover) ServeRequest(req AsyncApprovalRequest) {
	real := a.underlying

	ctx := req.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	dec, err := real.AskApproval(ctx, req.Request)
	req.ReplyCh <- AsyncApprovalResponse{Decision: dec, Err: err}
}

func (a *AsyncApprover) setThreadState(ctx context.Context,
	state ThreadState) {

	thread, ok := GetThread(ctx)
	if !ok || thread == nil {
		return
	}
	thread.SetState(state)
}
