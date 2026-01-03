/* Copyright © 2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package types

import (
	laclopenai "github.com/cloudwego/eino-ext/libs/acl/openai"
	"github.com/mikeb26/gptcli/internal/am"
)

type InternalContext struct {
	LlmVendor       string
	LlmModel        string
	LlmApiKey       string
	LlmAuditLogPath string
	// LlmReasoningEffort is a best-effort hint to the LLM client about how much
	// reasoning to apply. Expected values are "low", "medium", or "high".
	//
	// This is interpreted by llmclient.NewEINOClient and may be ignored by some
	// vendors/models.
	LlmReasoningEffort laclopenai.ReasoningEffortLevel
	LlmBaseApprover    am.Approver
}
