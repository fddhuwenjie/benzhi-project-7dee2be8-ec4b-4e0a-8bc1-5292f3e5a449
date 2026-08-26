package store

import (
	"encoding/json"
	"fmt"
	"os"
	"seed-vigor-gate/internal/domain"
	"sort"
)

func orderedAudit(trials map[string]*domain.VigorTrial) []domain.AuditEvent {
	var events []domain.AuditEvent
	for _, trial := range trials {
		events = append(events, trial.Audit...)
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].At.Equal(events[j].At) {
			return events[i].Digest < events[j].Digest
		}
		return events[i].At.Before(events[j].At)
	})
	return events
}

func (s *Store) auditWriterLocked() (*os.File, error) {
	if s.audit != nil {
		return s.audit, nil
	}
	file, err := os.OpenFile(s.auditPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	s.audit = file
	return file, nil
}

func (s *Store) recoverAuditLocked() error {
	wanted := orderedAudit(s.trials)
	existing, err := ReadAudit(s.auditPath())
	if err != nil {
		return err
	}
	if len(existing) > len(wanted) {
		return fmt.Errorf("audit log is ahead of snapshot")
	}
	for index, event := range existing {
		if event.Digest != wanted[index].Digest {
			return fmt.Errorf("audit log diverges at line %d", index+1)
		}
	}
	if len(existing) == len(wanted) {
		return nil
	}
	file, err := s.auditWriterLocked()
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	for _, event := range wanted[len(existing):] {
		if err = encoder.Encode(event); err != nil {
			return err
		}
	}
	if err = file.Sync(); err != nil {
		return err
	}
	return nil
}
