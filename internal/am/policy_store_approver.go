/* Copyright © 2023-2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package am

import (
	"context"
	"fmt"
)

type PolicyStoreApprover struct {
	underlying  Approver
	policyStore ApprovalPolicyStore
}

func NewPolicyStoreApprover(
	underlyingIn Approver,
	policyStoreIn ApprovalPolicyStore) *PolicyStoreApprover {

	return &PolicyStoreApprover{
		underlying:  underlyingIn,
		policyStore: policyStoreIn,
	}
}

func (psa *PolicyStoreApprover) AskApproval(ctx context.Context,
	req ApprovalRequest) (ApprovalDecision, error) {

	// If we have a policy store and this request declares required
	// actions, try to short-circuit based on cached policy. We only
	// consider choices that have a PolicyID (i.e. are eligible for
	// persistence) and whose cached action set is a superset of the
	// required actions.
	if psa.policyStore != nil && len(req.RequiredActions) > 0 {
		for _, ch := range req.Choices {
			if ch.PolicyID == "" {
				continue
			}
			if actions, found := psa.policyStore.Check(ch.PolicyID); found && hasAllApprovalActions(actions, req.RequiredActions) {
				return ApprovalDecision{
					Allowed: true,
					Choice:  ch,
				}, nil
			}
		}
	}

	if len(req.Choices) == 0 {
		return ApprovalDecision{}, fmt.Errorf("no approval choices provided")
	}

	chosen, err := psa.underlying.AskApproval(ctx, req)
	if err != nil {
		return chosen, err
	}

	// Persist policy when appropriate. We currently only persist allow
	// decisions that specify at least one action. Denials are not
	// persisted, so the user will be prompted again on subsequent
	// invocations.
	if psa.policyStore != nil && chosen.Choice.PolicyID != "" &&
		chosen.Choice.Scope == ApprovalScopeTarget &&
		chosen.Allowed && len(chosen.Choice.Actions) > 0 {
		psa.policyStore.Save(chosen.Choice.PolicyID, chosen.Choice.Actions)
	}

	return chosen, nil
}

// hasAllApprovalActions reports whether "have" contains all actions in
// "need". Both slices are treated as sets; order and duplicates are
// ignored.
func hasAllApprovalActions(have, need []ApprovalAction) bool {
	if len(need) == 0 {
		return true
	}

	set := make(map[ApprovalAction]struct{}, len(have))
	for _, a := range have {
		set[a] = struct{}{}
	}
	for _, a := range need {
		if _, ok := set[a]; !ok {
			return false
		}
	}
	return true
}
