/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package llmclient

import (
	"context"
	"testing"
	"time"

	"github.com/mikeb26/gptcli/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestInvocationID_GetAndEnsure(t *testing.T) {
	ctx := context.Background()

	id, ok := GetInvocationID(ctx)
	assert.False(t, ok)
	assert.Empty(t, id)

	ctx2, id2 := EnsureInvocationID(ctx)
	assert.NotEmpty(t, id2)

	id3, ok := GetInvocationID(ctx2)
	assert.True(t, ok)
	assert.Equal(t, id2, id3)

	ctx3, id4 := EnsureInvocationID(ctx2)
	assert.Equal(t, ctx2, ctx3)
	assert.Equal(t, id2, id4)
}

func TestProgress_Subscribe_LateSubscriberGetsCurrent(t *testing.T) {
	client := &EINOAIClient{
		subs:    make(map[string][]chan types.ProgressEvent),
		current: make(map[string]types.ProgressEvent),
	}

	invID := "inv-1"
	expected := types.ProgressEvent{
		InvocationID: invID,
		Component:    types.ProgressComponentTool,
		Phase:        types.ProgressPhaseStart,
		Time:         time.Now(),
		DisplayText:  "hello",
	}

	client.current[invID] = expected

	ch := client.SubscribeProgress(invID)
	if !assert.NotNil(t, ch) {
		return
	}
	defer func() {
		client.UnsubscribeProgress(ch, invID)
	}()

	select {
	case got := <-ch:
		assert.Equal(t, expected.InvocationID, got.InvocationID)
		assert.Equal(t, expected.Component, got.Component)
		assert.Equal(t, expected.Phase, got.Phase)
		assert.Equal(t, expected.DisplayText, got.DisplayText)
	default:
		t.Fatalf("expected to receive initial progress event")
	}
}

func TestProgress_Subscribe_EmptyInvocationIDReturnsNil(t *testing.T) {
	client := &EINOAIClient{
		subs:    make(map[string][]chan types.ProgressEvent),
		current: make(map[string]types.ProgressEvent),
	}

	ch := client.SubscribeProgress("")
	assert.Nil(t, ch)
}

func TestProgress_Publish_DoesNotBlockOnSlowSubscriber(t *testing.T) {
	client := &EINOAIClient{
		subs:    make(map[string][]chan types.ProgressEvent),
		current: make(map[string]types.ProgressEvent),
	}

	invID := "inv-1"
	ch := client.SubscribeProgress(invID)
	if !assert.NotNil(t, ch) {
		return
	}
	defer client.UnsubscribeProgress(ch, invID)

	// Fill the subscriber buffer to capacity so publishProgress has to hit the
	// non-blocking default case.
	for i := 0; i < 64; i++ {
		ch <- types.ProgressEvent{InvocationID: invID}
	}

	done := make(chan struct{})
	go func() {
		client.publishProgress(invID, types.ProgressEvent{InvocationID: invID, DisplayText: "x"})
		close(done)
	}()

	select {
	case <-done:
		// ok
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("publishProgress blocked")
	}
}

func TestProgress_Unsubscribe_ClosesChannel(t *testing.T) {
	client := &EINOAIClient{
		subs:    make(map[string][]chan types.ProgressEvent),
		current: make(map[string]types.ProgressEvent),
	}

	invID := "inv-1"
	ch := client.SubscribeProgress(invID)
	if !assert.NotNil(t, ch) {
		return
	}

	client.UnsubscribeProgress(ch, invID)

	_, ok := <-ch
	assert.False(t, ok)
}
