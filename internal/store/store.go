package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"seed-vigor-gate/internal/domain"
)

type Store struct {
	dir      string
	mu       sync.RWMutex
	trials   map[string]*domain.VigorTrial
	requests map[string]any
}

func New(dir string) (*Store, error) {
	s := &Store{dir: dir, trials: map[string]*domain.VigorTrial{}, requests: map[string]any{}}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}
func (s *Store) path() string      { return filepath.Join(s.dir, "snapshot.json") }
func (s *Store) auditPath() string { return filepath.Join(s.dir, "audit.jsonl") }
func (s *Store) load() error {
	b, err := os.ReadFile(s.path())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var xs []*domain.VigorTrial
	if err = json.Unmarshal(b, &xs); err != nil {
		return err
	}
	for _, t := range xs {
		s.trials[t.ID] = t
		for _, e := range t.Audit {
			if e.RequestID != "" {
				s.requests[e.RequestID] = e
			}
		}
	}
	if err = validateTrials(s.trials); err != nil {
		return err
	}
	return s.recoverAuditLocked()
}
func (s *Store) Get(id string) (*domain.VigorTrial, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.trials[id]
	if !ok {
		return nil, os.ErrNotExist
	}
	return t.Clone(), nil
}
func (s *Store) List() []*domain.VigorTrial {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*domain.VigorTrial, 0, len(s.trials))
	for _, t := range s.trials {
		out = append(out, t.Clone())
	}
	return out
}
func (s *Store) Seen(request string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.requests[request]
	return v, ok
}
func (s *Store) Save(t *domain.VigorTrial, request string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if request != "" {
		if _, ok := s.requests[request]; ok {
			return nil
		}
	}
	clone := t.Clone()
	previous, hadPrevious := s.trials[t.ID]
	s.trials[t.ID] = clone
	if request != "" {
		s.requests[request] = true
	}
	snapshotWritten, err := s.persistLocked()
	if err != nil {
		if !snapshotWritten {
			// The snapshot itself could not be replaced, so the on-disk state
			// is unchanged. Roll back the in-memory mutation so the failed
			// batch is neither queryable nor replayable; otherwise a retry
			// with the same request_id would return a phantom result that was
			// never persisted, and the batch would vanish after a restart.
			if hadPrevious {
				s.trials[t.ID] = previous
			} else {
				delete(s.trials, t.ID)
			}
			if request != "" {
				delete(s.requests, request)
			}
		}
		return err
	}
	return nil
}

// persistLocked writes the snapshot and reconciles the append-only audit log.
// The bool result reports whether the snapshot was durably written to disk;
// when false the on-disk snapshot is unchanged and callers must roll back any
// in-memory mutation that preceded the call.
func (s *Store) persistLocked() (bool, error) {
	xs := make([]*domain.VigorTrial, 0, len(s.trials))
	for _, x := range s.trials {
		xs = append(xs, x)
	}
	b, err := json.MarshalIndent(xs, "", "  ")
	if err != nil {
		return false, err
	}
	if err = atomicWrite(s.path(), b, 0644); err != nil {
		return false, err
	}
	if err = s.recoverAuditLocked(); err != nil {
		return true, err
	}
	return true, nil
}
func (s *Store) Timeline(id string) ([]domain.AuditEvent, error) {
	t, e := s.Get(id)
	if e != nil {
		return nil, e
	}
	return t.Audit, nil
}
func (s *Store) Validate() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := validateTrials(s.trials); err != nil {
		return err
	}
	events, err := ReadAudit(s.auditPath())
	if err != nil {
		return err
	}
	return validateAuditFile(events, s.trials)
}
func ReadAudit(path string) ([]domain.AuditEvent, error) {
	f, e := os.Open(path)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	defer f.Close()
	var out []domain.AuditEvent
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var x domain.AuditEvent
		if err := json.Unmarshal(sc.Bytes(), &x); err != nil {
			return nil, fmt.Errorf("invalid audit line: %w", err)
		}
		out = append(out, x)
	}
	return out, sc.Err()
}
