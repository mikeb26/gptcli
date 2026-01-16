/* Copyright © 2023-2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package am

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// JSONApprovalPolicyStore is an ApprovalPolicyStore implementation that
// persists policies to a JSON file on disk. Its lookup semantics are
// identical to MemoryApprovalPolicyStore, including recursive
// directory-scoped policies and "most specific ancestor" selection.
//
// The JSON file is a simple object mapping policy IDs to their allowed
// actions, e.g.:
//
//	{
//	  "tools:fileio:file:/tmp/foo.txt": ["read"],
//	  "tools:fileio:directory:/tmp/project": ["read", "write"]
//	}
type JSONApprovalPolicyStore struct {
	mu   sync.RWMutex
	file string
	data map[string][]ApprovalAction
}

// NewJSONApprovalPolicyStore constructs a JSON-backed policy store using
// the provided filename as the persistence location. If the file
// exists, it is read and parsed; if it does not exist, an empty store
// is initialized. Parent directories are not automatically created.
//
// The constructor is intentionally conservative: if the file exists but
// cannot be read or parsed, an error is returned rather than silently
// discarding existing data.
func NewJSONApprovalPolicyStore(filename string) (*JSONApprovalPolicyStore, error) {
	if filename == "" {
		return nil, errors.New("json policy store filename must not be empty")
	}

	store := &JSONApprovalPolicyStore{
		file: filename,
		data: make(map[string][]ApprovalAction),
	}

	// Best-effort load of existing data. We do this before taking the
	// mutex since the store is not yet published to other goroutines.
	if err := store.loadFromFile(); err != nil {
		return nil, err
	}

	return store, nil
}

// loadFromFile populates s.data from the JSON file if it exists.
// Missing files are treated as an empty store.
func (s *JSONApprovalPolicyStore) loadFromFile() error {
	path := s.file
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Nothing to load yet; start empty.
			return nil
		}
		return err
	}
	if info.IsDir() {
		return errors.New("json policy store path is a directory, want file")
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()

	var raw map[string][]ApprovalAction
	if err := dec.Decode(&raw); err != nil {
		return err
	}
	// A second decode should hit EOF; ignore that if so.
	_ = dec.Decode(new(any))

	if raw == nil {
		raw = make(map[string][]ApprovalAction)
	}
	s.data = raw
	return nil
}

// persist writes the current in-memory map to disk as JSON. The write
// is performed atomically via a temporary file + rename.
func (s *JSONApprovalPolicyStore) persist() error {
	// Marshal while holding the lock to get a consistent snapshot; this
	// is acceptable since the data set is expected to be small.
	encoded, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}

	// Ensure the target directory exists; if it doesn't, propagate the
	// error so the caller can surface it appropriately.
	dir := filepath.Dir(s.file)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	tmpPath := s.file + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(encoded); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}

	return os.Rename(tmpPath, s.file)
}

// Check implements ApprovalPolicyStore.Check with the same semantics as
// MemoryApprovalPolicyStore.
func (s *JSONApprovalPolicyStore) Check(policyID string) ([]ApprovalAction, bool) {
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

// Save implements ApprovalPolicyStore.Save. It updates the in-memory
// map and then attempts to persist to disk. If persistence fails, the
// in-memory update is still visible to subsequent calls.
func (s *JSONApprovalPolicyStore) Save(policyID string, actions []ApprovalAction) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data[policyID] = actions
	_ = s.persist()
}
