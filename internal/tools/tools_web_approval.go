/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package tools

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/mikeb26/gptcli/internal/am"
	"github.com/mikeb26/gptcli/internal/types"
)

// buildWebApprovalRequest is a shared helper for web-oriented tools
// (e.g., url_retrieve and url_render) that need consistent approval
// behavior with per-URL and per-domain caching.
//
// The provided method is normalized to upper-case; if empty, it
// defaults to GET. Methods GET, HEAD, and OPTIONS are treated as
// read-only; all others are considered write (and thus also imply
// read).
func buildWebApprovalRequest(t types.Tool, arg any, rawURL, method string) am.ApprovalRequest {
	// Parse the URL to extract a stable origin/domain component for
	// domain-scoped policies. If parsing fails, fall back to the
	// default approval behavior to avoid mis-caching.
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return DefaultApprovalRequest(t, arg)
	}

	m := strings.ToUpper(strings.TrimSpace(method))
	if m == "" {
		m = "GET"
	}

	// Treat safe/idempotent methods GET, HEAD, and OPTIONS as reads;
	// everything else is considered a write (and thus also implies
	// read).
	writeRequired := !(m == "GET" || m == "HEAD" || m == "OPTIONS")

	// Construct policy identifiers. For the per-URL policy we use the
	// full URL string. For the per-domain policy we use scheme://host
	// so that all paths under that origin share the same policy.
	urlPolicyID := am.ApprovalPolicyID(am.ApprovalSubsysTools,
		am.ApprovalGroupWeb, am.ApprovalTargetUrl, rawURL)
	domainKey := fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
	domainPolicyID := am.ApprovalPolicyID(am.ApprovalSubsysTools,
		am.ApprovalGroupWeb, am.ApprovalTargetDomain, domainKey)

	promptBuilder := &strings.Builder{}
	promptBuilder.WriteString(fmt.Sprintf("gptcli would like to %v(%v): %v. Allow?",
		t.GetOp(), m, rawURL))

	choices := []am.ApprovalChoice{
		{
			Key:   "y",
			Label: "Yes, this time only",
			Scope: am.ApprovalScopeOnce,
		},
	}

	// Read-only caching options (GET requests only).
	if !writeRequired {
		choices = append(choices,
			am.ApprovalChoice{
				Key:      "ur",
				Label:    "Yes, and allow all future reads (GET) from this URL",
				Scope:    am.ApprovalScopeTarget,
				PolicyID: urlPolicyID,
				Actions:  []am.ApprovalAction{am.ApprovalActionRead},
			},
			am.ApprovalChoice{
				Key:      "dr",
				Label:    "Yes, and allow all future reads (GET) from this domain",
				Scope:    am.ApprovalScopeTarget,
				PolicyID: domainPolicyID,
				Actions:  []am.ApprovalAction{am.ApprovalActionRead},
			},
		)
	}

	// Read/write caching options intended for state-changing operations
	// (non-GET), but also available to strongly trust a given URL or
	// domain for both reads and writes.
	choices = append(choices,
		am.ApprovalChoice{
			Key:      "uw",
			Label:    "Yes, and allow all future reads/writes (GET/POST) to this URL",
			Scope:    am.ApprovalScopeTarget,
			PolicyID: urlPolicyID,
			Actions: []am.ApprovalAction{am.ApprovalActionWrite,
				am.ApprovalActionRead},
		},
		am.ApprovalChoice{
			Key:      "dw",
			Label:    "Yes, and allow all future reads/writes (GET/POST) for this domain",
			Scope:    am.ApprovalScopeTarget,
			PolicyID: domainPolicyID,
			Actions: []am.ApprovalAction{am.ApprovalActionWrite,
				am.ApprovalActionRead},
		},
	)

	choices = append(choices, am.ApprovalChoice{
		Key:   "n",
		Label: "No",
		Scope: am.ApprovalScopeDeny,
	})

	required := []am.ApprovalAction{am.ApprovalActionRead}
	if writeRequired {
		required = append(required, am.ApprovalActionWrite)
	}

	return am.ApprovalRequest{
		Prompt:          promptBuilder.String(),
		RequiredActions: required,
		Choices:         choices,
	}
}
