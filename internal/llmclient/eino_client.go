/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package llmclient

import (
	"context"
	"fmt"
	"sync"

	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino-ext/components/model/openai"
	laclopenai "github.com/cloudwego/eino-ext/libs/acl/openai"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/mikeb26/gptcli/internal"
	"github.com/mikeb26/gptcli/internal/am"
	"github.com/mikeb26/gptcli/internal/tools"
	"github.com/mikeb26/gptcli/internal/types"
	"google.golang.org/genai"
)

type EINOAIClient struct {
	reactAgent      *react.Agent
	reasoningEffort laclopenai.ReasoningEffortLevel
	auditHandler    callbacks.Handler
	statusHandlers  callbacks.Handler

	approver am.Approver

	subsMu sync.RWMutex
	subs   map[string][]chan types.ProgressEvent //index by invocationID

	// current holds the most recent progress event per invocation ID so that
	// late subscribers (e.g. UI subscribing after Stream() returns) can still
	// learn what is currently happening.
	currentMu sync.RWMutex
	current   map[string]types.ProgressEvent
}

// invocationIDKey is an unexported context key type used to store a per-
// invocation ID so that all audit log entries for a single originating call
// to CreateChatCompletion / StreamChatCompletion can be correlated.
type invocationIDKey struct{}

// GetInvocationID extracts the invocation ID from the context, if present.
func GetInvocationID(ctx context.Context) (string, bool) {
	if v := ctx.Value(invocationIDKey{}); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s, true
		}
	}
	return "", false
}

// ensureInvocationID returns a context that is guaranteed to carry an
// invocation ID, and the ID itself. If the ID is already present, it is
// reused; otherwise, a new UUID is generated and attached to the context.
func EnsureInvocationID(ctx context.Context) (context.Context, string) {
	if id, ok := GetInvocationID(ctx); ok {
		return ctx, id
	}
	id := uuid.NewString()
	ctx = context.WithValue(ctx, invocationIDKey{}, id)
	return ctx, id
}

func NewEINOClient(ctx context.Context, ictx types.InternalContext,
	approver am.Approver, depth int) types.AIClient {

	vendor := ictx.LlmVendor
	apiKey := ictx.LlmApiKey
	modelName := ictx.LlmModel
	enableAuditLog := ictx.LlmAuditLogPath != ""
	auditLogPath := ictx.LlmAuditLogPath

	var client types.AIClient
	switch vendor {
	case "openai":
		client = newOpenAIEINOClient(ctx, vendor, approver, apiKey, modelName, depth,
			enableAuditLog, auditLogPath)
	case "anthropic":
		client = newAnthropicEINOClient(ctx, vendor, approver, apiKey, modelName, depth,
			enableAuditLog, auditLogPath)
	case "google":
		client = newGoogleEINOClient(ctx, vendor, approver, apiKey, modelName, depth,
			enableAuditLog, auditLogPath)
	default:
		panic("unsupported vendor")
	}

	client.SetReasoning(ictx.LlmReasoningEffort)

	return client
}

func newOpenAIEINOClient(ctx context.Context, vendor string,
	approver am.Approver, apiKey string, model string,
	depth int,
	enableAuditLog bool, auditLogPath string) types.AIClient {

	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		Model:  model,
		APIKey: apiKey,
	})
	if err != nil {
		panic(err)
	}

	return newEINOClient(ctx, vendor, chatModel, approver, apiKey, model, depth,
		enableAuditLog, auditLogPath)
}

func newAnthropicEINOClient(ctx context.Context, vendor string,
	approver am.Approver, apiKey string, model string,
	depth int,
	enableAuditLog bool, auditLogPath string) types.AIClient {

	chatModel, err := claude.NewChatModel(ctx, &claude.Config{
		Model:  model,
		APIKey: apiKey,
		// currently hardcode max tokens to 64k; see
		// https://platform.claude.com/docs/en/api/go/messages/create
		// https://platform.claude.com/docs/en/about-claude/models/overview
		MaxTokens: 64000,
	})
	if err != nil {
		panic(err)
	}

	return newEINOClient(ctx, vendor, chatModel, approver, apiKey, model, depth,
		enableAuditLog, auditLogPath)
}

func newGoogleEINOClient(ctx context.Context, vendor string,
	approver am.Approver, apiKey string, model string,
	depth int,
	enableAuditLog bool, auditLogPath string) types.AIClient {

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		panic(err)
	}

	chatModel, err := gemini.NewChatModel(ctx, &gemini.Config{
		Model:  model,
		Client: client,
	})
	if err != nil {
		panic(err)
	}

	return newEINOClient(ctx, vendor, chatModel, approver, apiKey, model, depth,
		enableAuditLog, auditLogPath)
}

func newEINOClient(ctx context.Context, vendor string, chatModel model.ChatModel,
	approver am.Approver, apiKey string, model string,
	depth int, enableAuditLog bool, auditLogPath string) types.AIClient {

	tools := defineTools(ctx, vendor, approver, apiKey, model, depth)
	baseTools := make([]tool.BaseTool, len(tools))
	for ii, _ := range tools {
		baseTools[ii] = tools[ii]
	}
	config := &react.AgentConfig{
		Model:   chatModel,
		MaxStep: 250,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: baseTools,
		},
	}

	client, err := react.NewAgent(ctx, config)
	if err != nil {
		panic(err)
	}

	var auditHandler callbacks.Handler
	if enableAuditLog {
		auditHandler, err = newAuditCallbacksHandler(auditLogPath)
		if err != nil {
			panic(err)
		}
	}

	clientOut := &EINOAIClient{
		reactAgent:      client,
		reasoningEffort: laclopenai.ReasoningEffortLevelMedium,
		auditHandler:    auditHandler,
		approver:        approver,
		subs:            make(map[string][]chan types.ProgressEvent),
		current:         make(map[string]types.ProgressEvent),
	}
	clientOut.statusHandlers = newStatusCallbackHandlers(clientOut)
	return clientOut
}

func defineTools(ctx context.Context, vendor string, approver am.Approver,
	apiKey string, model string, depth int) []types.LlmTool {

	tools := []types.LlmTool{
		tools.NewRunCommandTool(approver),
		tools.NewCreateFileTool(approver),
		tools.NewAppendFileTool(approver),
		tools.NewFilePatchTool(approver),
		tools.NewReadFileTool(approver),
		tools.NewDeleteFileTool(approver),
		tools.NewPwdTool(approver),
		tools.NewChdirTool(approver),
		tools.NewEnvGetTool(approver),
		tools.NewEnvSetTool(approver),
		tools.NewRetrieveUrlTool(approver),
	}
	if depth <= internal.MaxDepth {
		tools = append(tools, newPromptRunTool(ctx, vendor, approver, apiKey,
			model, depth))
	}

	return tools
}

func (client *EINOAIClient) SetReasoning(
	reasoningEffort laclopenai.ReasoningEffortLevel) {
	client.reasoningEffort = reasoningEffort
}

func (client *EINOAIClient) CreateChatCompletion(ctx context.Context,
	dialogueIn []*types.ThreadMessage) (*types.ThreadMessage, error) {

	// Ensure this invocation has a correlation ID for audit/progress callbacks.
	// If an ID is already present in the context (e.g. set by a higher-level
	// caller), it will be reused; otherwise, a new one is generated.
	ctx, _ = EnsureInvocationID(ctx)

	dialogue := make([]*schema.Message, len(dialogueIn))
	for ii, msg := range dialogueIn {
		dialogue[ii] = (*schema.Message)(msg)
	}

	modelOpt := laclopenai.WithReasoningEffort(client.reasoningEffort)
	composeOpt := compose.WithChatModelOption(modelOpt)
	agentOpt := agent.WithComposeOptions(composeOpt)

	// attach callbacks for model and tool invocations
	var cbComposeOpt compose.Option
	if client.auditHandler != nil {
		cbComposeOpt = compose.WithCallbacks(client.auditHandler,
			client.statusHandlers)
	} else {
		cbComposeOpt = compose.WithCallbacks(client.statusHandlers)
	}
	cbAgentOpt := agent.WithComposeOptions(cbComposeOpt)

	msg, err := client.reactAgent.Generate(ctx, dialogue, agentOpt,
		cbAgentOpt)
	return (*types.ThreadMessage)(msg), err
}

func (client *EINOAIClient) StreamChatCompletion(ctx context.Context,
	dialogueIn []*types.ThreadMessage) (*types.StreamResult, error) {

	// Ensure this invocation has a correlation ID for audit/progress callbacks.
	// If the caller already attached an ID to ctx, we will reuse it.
	ctx, invocationID := EnsureInvocationID(ctx)

	dialogue := make([]*schema.Message, len(dialogueIn))
	for ii, msg := range dialogueIn {
		dialogue[ii] = (*schema.Message)(msg)
	}

	modelOpt := laclopenai.WithReasoningEffort(client.reasoningEffort)
	composeOpt := compose.WithChatModelOption(modelOpt)
	agentOpt := agent.WithComposeOptions(composeOpt)

	// attach callbacks for model and tool invocations
	var cbComposeOpt compose.Option
	if client.auditHandler != nil {
		cbComposeOpt = compose.WithCallbacks(client.auditHandler,
			client.statusHandlers)
	} else {
		cbComposeOpt = compose.WithCallbacks(client.statusHandlers)
	}
	cbAgentOpt := agent.WithComposeOptions(cbComposeOpt)

	stream, err := client.reactAgent.Stream(ctx, dialogue, agentOpt, cbAgentOpt)
	if err != nil {
		return nil, err
	}

	convert := func(m *schema.Message) (*types.ThreadMessage, error) {
		if m == nil {
			return nil, fmt.Errorf("nil message in stream")
		}
		return (*types.ThreadMessage)(m), nil
	}

	streamOut := schema.StreamReaderWithConvert(stream, convert)
	return &types.StreamResult{
		InvocationID: invocationID,
		Stream:       streamOut,
	}, nil
}

// SubscribeProgress registers a subscriber for callback-driven progress events
// for the given invocation ID.
//
// The returned channel will receive events best-effort; if the receiver is too
// slow, events may be dropped. It is the caller's responsibility to call
// UnsubscribeProcess() when no longer required.
func (client *EINOAIClient) SubscribeProgress(
	invocationID string) chan types.ProgressEvent {

	ch := make(chan types.ProgressEvent, 64)
	if invocationID == "" {
		close(ch)
		return nil
	}

	client.subsMu.Lock()
	client.subs[invocationID] = append(client.subs[invocationID], ch)
	client.subsMu.Unlock()

	// Best-effort send the most recent known status for this invocation so the
	// caller doesn't miss early tool/model events that may have fired before the
	// subscription was established.
	client.currentMu.RLock()
	if ev, ok := client.current[invocationID]; ok {
		select {
		case ch <- ev:
		default:
		}
	}
	client.currentMu.RUnlock()

	return ch
}

// UnsubscribeProgress unregisters a subscriber from a previously subscribed
// invocationID
func (client *EINOAIClient) UnsubscribeProgress(ch chan types.ProgressEvent,
	invocationID string) {

	client.subsMu.Lock()
	defer client.subsMu.Unlock()

	subs := client.subs[invocationID]
	for i := range subs {
		if subs[i] == ch {
			subs = append(subs[:i], subs[i+1:]...)
			close(ch)
			break
		}
	}
	if len(subs) == 0 {
		delete(client.subs, invocationID)
	} else {
		client.subs[invocationID] = subs
	}
}

func (client *EINOAIClient) publishProgress(invocationID string, ev types.ProgressEvent) {
	if invocationID == "" {
		return
	}

	// Store the latest event so late subscribers can catch up.
	client.currentMu.Lock()
	client.current[invocationID] = ev
	client.currentMu.Unlock()

	subs := make([]chan types.ProgressEvent, 0)

	// make a local copy of the set of subscribers so that new subscribers
	// don't race with iteration
	client.subsMu.RLock()
	subs = append(subs, client.subs[invocationID]...)
	client.subsMu.RUnlock()

	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
			// drop if subscriber is slow
		}
	}
}
