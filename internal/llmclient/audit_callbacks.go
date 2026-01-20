/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package llmclient

import (
	"context"
	"io"
	"log"
	"os"
	"strings"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	ub "github.com/cloudwego/eino/utils/callbacks"
)

// summarizeText returns a truncated version of s for logging purposes.
func summarizeText(s string) string {
	const maxLen = 200
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// summarizeMessages produces a compact textual representation of a slice of
// schema.Message values suitable for audit logging.
func summarizeMessages(msgs []*schema.Message) string {
	if len(msgs) == 0 {
		return "<no-messages>"
	}

	// Only summarize the last message in the dialogue for logging.
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m == nil {
			continue
		}

		// If the last non-nil message is a tool message, this callback
		// invocation is just the agent sending tool responses back to the
		// model. In that case, avoid logging the raw JSON payload and use a
		// compact sentinel instead.
		if m.Role == schema.Tool {
			return "<tool responses>"
		}

		var b strings.Builder
		b.WriteString(string(m.Role))
		b.WriteString(": ")
		b.WriteString(summarizeText(m.Content))
		return b.String()
	}

	return "<no-messages>"
}

// getInvocationIDForLog builds the textual prefix for audit log lines based on
// the invocation ID stored in the context, if any.
func getInvocationIDForLog(ctx context.Context) string {
	id := GetInvocationID(ctx)
	if id != "" {
		return "[" + id + "] "
	}

	return ""
}

// getRunName resolves the effective name for a callback run, falling back to
// defaultName when the callbacks.RunInfo is nil or has an empty Name.
func getRunName(defaultName string, info *callbacks.RunInfo) string {
	if info != nil && info.Name != "" {
		return info.Name
	}
	return defaultName
}

type auditModelCallbacks struct {
	logger *log.Logger
}

func (h *auditModelCallbacks) OnStart(
	ctx context.Context,
	info *callbacks.RunInfo,
	input *model.CallbackInput,
) context.Context {
	name := getRunName("chat_model", info)

	argsSummary := "<nil>"
	if input != nil {
		argsSummary = summarizeMessages(input.Messages)
	}

	prefix := getInvocationIDForLog(ctx)
	h.logger.Printf("%smodel_%s: %s start", prefix, name, argsSummary)
	return ctx
}

func (h *auditModelCallbacks) OnEnd(
	ctx context.Context,
	info *callbacks.RunInfo,
	output *model.CallbackOutput,
) context.Context {
	name := getRunName("chat_model", info)

	// Log the main assistant content in a summarized form.
	resp := "<nil>"
	var reasoning string
	if output != nil && output.Message != nil {
		if output.Message.Content != "" {
			resp = summarizeText(output.Message.Content)
		}
		if rc := output.Message.ReasoningContent; rc != "" {
			reasoning = rc
		}
	}

	prefix := getInvocationIDForLog(ctx)
	h.logger.Printf("%smodel_%s: %s end", prefix, name, resp)

	// If reasoning content is present, log it on a dedicated line without
	// truncation so the full chain-of-thought is available for audits.
	if reasoning != "" {
		h.logger.Printf("%smodel_%s: reasoning: %s", prefix, name, reasoning)
	}
	return ctx
}

func (h *auditModelCallbacks) OnEndWithStreamOutput(
	ctx context.Context,
	info *callbacks.RunInfo,
	output *schema.StreamReader[*model.CallbackOutput],
) context.Context {
	name := getRunName("chat_model", info)

	// For streaming outputs, consume the callback stream on a separate
	// goroutine so we can log both intermediate chunks and a summarized
	// final response (plus any reasoning content) without interfering
	// with the main response stream returned to the UI.
	if output != nil {
		prefix := getInvocationIDForLog(ctx)
		go h.drainModelStream(prefix, name, output)
	}

	return ctx
}

func (h *auditModelCallbacks) drainModelStream(
	prefix, name string,
	sr *schema.StreamReader[*model.CallbackOutput],
) {
	defer sr.Close()

	resp := "<nil>"
	var reasoningSb strings.Builder
	var contentSb strings.Builder
	started := false

	for {
		chunk, err := sr.Recv()
		if err != nil {
			if err != io.EOF {
				h.logger.Printf("%smodel_%s: <streaming> error: %v", prefix, name, err)
			}
			break
		}

		if chunk == nil || chunk.Message == nil {
			continue
		}

		msg := chunk.Message
		if msg.Content != "" {
			if !started {
				h.logger.Printf("%smodel_%s: <streaming> start", prefix, name)
				started = true
			}
			contentSb.WriteString(msg.Content)
		}
		if rc := msg.ReasoningContent; rc != "" {
			reasoningSb.WriteString(rc)
		}
	}

	resp = summarizeText(reasoningSb.String())
	if resp != "" {
		h.logger.Printf("%smodel_%s: reasoning:%s", prefix, name,
			resp)
	}
	resp = summarizeText(contentSb.String())
	if resp != "" {
		h.logger.Printf("%smodel_%s: <streaming> end resp:%s", prefix, name,
			resp)
	}
}

type auditToolCallbacks struct {
	logger *log.Logger
}

func (h *auditToolCallbacks) OnStart(
	ctx context.Context,
	info *callbacks.RunInfo,
	input *tool.CallbackInput,
) context.Context {
	name := getRunName("tool", info)

	args := "<nil>"
	if input != nil {
		args = summarizeText(input.ArgumentsInJSON)
	}

	prefix := getInvocationIDForLog(ctx)
	h.logger.Printf("%stool_%s: %s start", prefix, name, args)
	return ctx
}

func (h *auditToolCallbacks) OnEnd(
	ctx context.Context,
	info *callbacks.RunInfo,
	output *tool.CallbackOutput,
) context.Context {
	name := getRunName("tool", info)

	resp := "<nil>"
	if output != nil {
		resp = summarizeText(output.Response)
	}

	prefix := getInvocationIDForLog(ctx)
	h.logger.Printf("%stool_%s: %s end", prefix, name, resp)
	return ctx
}

func (h *auditToolCallbacks) OnEndWithStreamOutput(
	ctx context.Context,
	info *callbacks.RunInfo,
	output *schema.StreamReader[*tool.CallbackOutput],
) context.Context {
	name := getRunName("tool", info)

	// For streaming tool outputs, asynchronously drain the callback
	// stream and log intermediate chunks plus a summarized final
	// response.
	if output != nil {
		prefix := getInvocationIDForLog(ctx)
		go h.drainToolStream(prefix, name, output)
	}

	return ctx
}

func (h *auditToolCallbacks) drainToolStream(
	prefix, name string,
	sr *schema.StreamReader[*tool.CallbackOutput],
) {
	defer sr.Close()

	resp := "<nil>"

	for {
		chunk, err := sr.Recv()
		if err != nil {
			if err != io.EOF {
				h.logger.Printf("%stool_%s: <streaming> error: %v", prefix, name, err)
			}
			break
		}

		if chunk == nil {
			continue
		}

		if chunk.Response != "" {
			resp = summarizeText(chunk.Response)
			// Log each non-empty response chunk as it arrives so
			// that tool streaming activity is visible during long
			// runs.
			h.logger.Printf("%stool_%s: <streaming> %s", prefix, name, resp)
		}
	}

	h.logger.Printf("%stool_%s: %s end", prefix, name, resp)
}

// newAuditModelHandler constructs a ModelCallbackHandler that logs model
// invocations and responses using the provided logger.
func newAuditModelHandler(logger *log.Logger) *ub.ModelCallbackHandler {
	cb := &auditModelCallbacks{logger: logger}
	return &ub.ModelCallbackHandler{
		OnStart:               cb.OnStart,
		OnEnd:                 cb.OnEnd,
		OnEndWithStreamOutput: cb.OnEndWithStreamOutput,
	}
}

// newAuditToolHandler constructs a ToolCallbackHandler that logs tool
// invocations and responses using the provided logger.
func newAuditToolHandler(logger *log.Logger) *ub.ToolCallbackHandler {
	cb := &auditToolCallbacks{logger: logger}
	return &ub.ToolCallbackHandler{
		OnStart:               cb.OnStart,
		OnEnd:                 cb.OnEnd,
		OnEndWithStreamOutput: cb.OnEndWithStreamOutput,
	}
}

// newAuditCallbacksHandler builds a callbacks.Handler that wires the model and
// tool audit handlers into EINO's callback system.
func newAuditCallbacksHandler(logfile string) (callbacks.Handler, error) {
	f, err := os.OpenFile(logfile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}

	// Include a trailing space in the prefix so entries are easier to scan,
	// e.g. "gptcli 2025/01/02 15:04:05 [..." instead of "gptcli2025/...".
	logger := log.New(f, "gptcli ", log.LstdFlags)

	helper := ub.NewHandlerHelper().
		ChatModel(newAuditModelHandler(logger)).
		Tool(newAuditToolHandler(logger))

	return helper.Handler(), nil
}
