/* Copyright © 2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package types

import "github.com/mikeb26/gptcli/internal/am"

type InternalContext struct {
	LlmVendor       string
	LlmModel        string
	LlmApiKey       string
	LlmAuditLogPath string
	LlmBaseApprover am.Approver
}
