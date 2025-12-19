/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package llmclient

import (
	"context"
	"log"
	"strings"
	"testing"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
)

func TestSummarizeText(t *testing.T) {
	short := "hello"
	assert.Equal(t, short, summarizeText(short))

	long := strings.Repeat("a", 250)
	sum := summarizeText(long)
	assert.True(t, strings.HasSuffix(sum, "..."))
	assert.LessOrEqual(t, len(sum), 203)
}

func TestSummarizeMessages_LastToolMessage(t *testing.T) {
	msgs := []*schema.Message{
		{Role: schema.User, Content: "hi"},
		{Role: schema.Tool, Content: "{\"big\":\"json\"}"},
	}
	assert.Equal(t, "<tool responses>", summarizeMessages(msgs))
}

func TestSummarizeMessages_LastNonNil(t *testing.T) {
	msgs := []*schema.Message{
		nil,
		{Role: schema.User, Content: "hello"},
		{Role: schema.Assistant, Content: "world"},
	}
	assert.Equal(t, "assistant: world", summarizeMessages(msgs))
}

func TestGetInvocationIDForLog(t *testing.T) {
	ctx := context.Background()
	assert.Equal(t, "", getInvocationIDForLog(ctx))

	ctx2, id := EnsureInvocationID(ctx)
	assert.Equal(t, "["+id+"] ", getInvocationIDForLog(ctx2))
}

func TestGetRunName(t *testing.T) {
	assert.Equal(t, "tool", getRunName("tool", nil))
	assert.Equal(t, "tool", getRunName("tool", &callbacks.RunInfo{}))
	assert.Equal(t, "x", getRunName("tool", &callbacks.RunInfo{Name: "x"}))
}

func TestAuditModelCallbacks_OnStartAndEnd(t *testing.T) {
	buf := &strings.Builder{}
	logger := log.New(buf, "", 0)
	cb := &auditModelCallbacks{logger: logger}

	ctx, id := EnsureInvocationID(context.Background())

	cb.OnStart(ctx, &callbacks.RunInfo{Name: "m"}, &model.CallbackInput{
		Messages: []*schema.Message{{Role: schema.User, Content: "hello"}},
	})
	cb.OnEnd(ctx, &callbacks.RunInfo{Name: "m"}, &model.CallbackOutput{
		Message: &schema.Message{Role: schema.Assistant, Content: "world", ReasoningContent: "because"},
	})

	out := buf.String()
	assert.Contains(t, out, "["+id+"] model_m: user: hello start")
	assert.Contains(t, out, "["+id+"] model_m: world end")
	assert.Contains(t, out, "["+id+"] model_m: reasoning: because")
}

func TestAuditModelCallbacks_DrainModelStream(t *testing.T) {
	buf := &strings.Builder{}
	logger := log.New(buf, "", 0)
	cb := &auditModelCallbacks{logger: logger}

	sr := schema.StreamReaderFromArray([]*model.CallbackOutput{
		{Message: &schema.Message{Content: "chunk1"}},
		{Message: &schema.Message{Content: "chunk2", ReasoningContent: "r"}},
	})

	cb.drainModelStream("[id] ", "m", sr)

	out := buf.String()
	assert.Contains(t, out, "[id] model_m: <streaming> chunk1")
	assert.Contains(t, out, "[id] model_m: <streaming> chunk2")
	assert.Contains(t, out, "[id] model_m: chunk2 end")
	assert.Contains(t, out, "[id] model_m: reasoning: r")
}

func TestAuditToolCallbacks_DrainToolStream(t *testing.T) {
	buf := &strings.Builder{}
	logger := log.New(buf, "", 0)
	cb := &auditToolCallbacks{logger: logger}

	toolStream := schema.StreamReaderFromArray([]*einotool.CallbackOutput{
		{Response: "chunk1"},
		{Response: "chunk2"},
	})

	cb.drainToolStream("[id] ", "t", toolStream)

	out := buf.String()
	assert.Contains(t, out, "[id] tool_t: <streaming> chunk1")
	assert.Contains(t, out, "[id] tool_t: <streaming> chunk2")
	assert.Contains(t, out, "[id] tool_t: chunk2 end")
}
