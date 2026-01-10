/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package types

import (
	"time"
)

// ProgressPhase captures a high-level phase for callback-driven progress
// updates that can be consumed by the UI.
type ProgressPhase string

const (
	ProgressPhaseStart         ProgressPhase = "start"
	ProgressPhaseEnd           ProgressPhase = "end"
	ProgressPhaseStreamingResp ProgressPhase = "streaming"
)

// ProgressComponent indicates whether the event is describing model or tool
// activity.
type ProgressComponent string

const (
	ProgressComponentModel ProgressComponent = "model"
	ProgressComponentTool  ProgressComponent = "tool"
)

// ProgressEvent is emitted from EINO callbacks and routed to subscribers using
// the invocation ID.
type ProgressEvent struct {
	InvocationID string
	Component    ProgressComponent
	Phase        ProgressPhase
	Time         time.Time
	DisplayText  string
}
