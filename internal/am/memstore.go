/* Copyright © 2023-2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package am

import (
	"sync"
)

// MemoryApprovalPolicyStore is an in-memory implementation of
// ApprovalPolicyStore guarded by a mutex. It is intentionally simple for
// now and can be extended later to support persistence.
type MemoryApprovalPolicyStore struct {
	mu   sync.RWMutex
	data map[string][]ApprovalAction
}

// NewMemoryApprovalPolicyStore constructs a ready-to-use in-memory policy
// store.
func NewMemoryApprovalPolicyStore() *MemoryApprovalPolicyStore {
	return &MemoryApprovalPolicyStore{
		data: make(map[string][]ApprovalAction),
	}
}

func (s *MemoryApprovalPolicyStore) Check(policyID string) ([]ApprovalAction, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Fast path: exact match
	if val, ok := s.data[policyID]; ok {
		return val, true
	}

	// For directory-scoped policies, we support recursive semantics: an
	// approval for a directory applies to that directory and all of its
	// subdirectories. To implement this, when asked to check a directory
	// policy we look for any stored policy whose domain is an ancestor of
	// the requested directory.
	subsys, group, target, domain, ok := parsePolicyID(policyID)
	if !ok || (target != ApprovalTargetDir && target != ApprovalTargetDomain) {
		return nil, false
	}

	var (
		bestMatchLen int = -1
		bestActions  []ApprovalAction
		found        bool
	)
	for storedID, actions := range s.data {
		ss, sg, st, sdomain, ok := parsePolicyID(storedID)
		if !ok {
			continue
		}
		if ss != subsys || sg != group || st != target {
			continue
		}
		if !isPathWithin(domain, sdomain) {
			continue
		}
		if l := len(sdomain); l > bestMatchLen {
			bestMatchLen = l
			bestActions = actions
			found = true
		}
	}
	if found {
		return bestActions, true
	}

	return nil, false
}

func (s *MemoryApprovalPolicyStore) Save(policyID string, actions []ApprovalAction) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Replace semantics: the stored action set becomes exactly the
	// provided slice. Callers should treat the slice as immutable after
	// passing it here.
	s.data[policyID] = actions
}
