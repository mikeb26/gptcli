/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package threads

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mikeb26/gptcli/internal/am"
)

type mockApprover struct {
	askFn func(ctx context.Context, req am.ApprovalRequest) (am.ApprovalDecision, error)
}

func (m *mockApprover) AskApproval(ctx context.Context, req am.ApprovalRequest) (am.ApprovalDecision, error) {
	if m.askFn == nil {
		return am.ApprovalDecision{}, errors.New("AskApproval not implemented")
	}
	return m.askFn(ctx, req)
}

func TestAsyncApprover_ClosePreventsNewRequests(t *testing.T) {
	a := NewAsyncApprover(&mockApprover{})
	a.Close()

	_, err := a.AskApproval(context.Background(), am.ApprovalRequest{Prompt: "x", Choices: []am.ApprovalChoice{{Key: "y"}}})
	if err == nil {
		t.Fatalf("expected error from AskApproval after Close")
	}

	select {
	case req := <-a.Requests:
		t.Fatalf("did not expect any request sent after Close, got: %+v", req)
	case <-time.After(30 * time.Millisecond):
		// ok
	}
}

func TestAsyncApprover_ForwardsRequestAndReturnsResponse(t *testing.T) {
	a := NewAsyncApprover(&mockApprover{askFn: func(ctx context.Context, req am.ApprovalRequest) (am.ApprovalDecision, error) {
		if req.Prompt != "approve?" {
			return am.ApprovalDecision{}, errors.New("unexpected prompt")
		}
		return am.ApprovalDecision{Allowed: true, Choice: am.ApprovalChoice{Key: "y"}}, nil
	}})

	done := make(chan struct{})
	go func() {
		defer close(done)
		dec, err := a.AskApproval(context.Background(), am.ApprovalRequest{Prompt: "approve?", Choices: []am.ApprovalChoice{{Key: "y"}}})
		if err != nil {
			t.Errorf("AskApproval returned error: %v", err)
			return
		}
		if !dec.Allowed {
			t.Errorf("expected Allowed=true")
		}
		if dec.Choice.Key != "y" {
			t.Errorf("expected choice key 'y', got %q", dec.Choice.Key)
		}
	}()

	select {
	case req := <-a.Requests:
		if req.Request.Prompt != "approve?" {
			t.Fatalf("expected Prompt 'approve?', got %q", req.Request.Prompt)
		}
		if req.ReplyCh == nil {
			t.Fatalf("expected non-nil ReplyCh")
		}
		a.ServeRequest(req)
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("timed out waiting for async approval request")
	}

	select {
	case <-done:
		// ok
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("timed out waiting for AskApproval to return")
	}
}

func TestAsyncApprover_ContextCanceledBeforeSendReturnsContextError(t *testing.T) {
	a := NewAsyncApprover(&mockApprover{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := a.AskApproval(ctx, am.ApprovalRequest{Prompt: "x", Choices: []am.ApprovalChoice{{Key: "y"}}})
	if err == nil {
		t.Fatalf("expected context error")
	}
}

func TestAsyncApprover_ContextCanceledWhileWaitingForReplyReturnsContextError(t *testing.T) {
	a := NewAsyncApprover(&mockApprover{})
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		_, err := a.AskApproval(ctx, am.ApprovalRequest{Prompt: "x", Choices: []am.ApprovalChoice{{Key: "y"}}})
		errCh <- err
	}()

	// Receive the request to ensure AskApproval has sent it.
	select {
	case <-a.Requests:
		// now cancel before replying
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("timed out waiting for request")
	}
	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatalf("expected context error")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("timed out waiting for AskApproval to return")
	}
}

func TestAsyncApprover_ServeRequest_DispatchesToUnderlyingApproverAndReplies(t *testing.T) {
	m := &mockApprover{
		askFn: func(ctx context.Context, req am.ApprovalRequest) (am.ApprovalDecision, error) {
			if req.Prompt != "pick" {
				return am.ApprovalDecision{}, errors.New("wrong prompt")
			}
			return am.ApprovalDecision{Allowed: true, Choice: am.ApprovalChoice{Key: "y"}}, nil
		},
	}

	a := NewAsyncApprover(m)
	replyCh := make(chan AsyncApprovalResponse, 1)
	a.ServeRequest(AsyncApprovalRequest{Ctx: context.Background(), Request: am.ApprovalRequest{Prompt: "pick", Choices: []am.ApprovalChoice{{Key: "y"}}}, ReplyCh: replyCh})
	resp := <-replyCh
	if resp.Err != nil {
		t.Fatalf("expected no error, got %v", resp.Err)
	}
	if !resp.Decision.Allowed {
		t.Fatalf("expected Allowed=true")
	}
}
