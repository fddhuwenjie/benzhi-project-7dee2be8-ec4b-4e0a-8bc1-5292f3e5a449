package store

import (
	"fmt"
	"seed-vigor-gate/internal/domain"
)

func validateTrials(trials map[string]*domain.VigorTrial) error {
	for id, trial := range trials {
		if id != trial.ID {
			return fmt.Errorf("snapshot key %s differs from trial id %s", id, trial.ID)
		}
		if err := trial.ValidateIdentity(); err != nil {
			return fmt.Errorf("trial %s identity: %w", id, err)
		}
		if err := trial.VerifyIntegrity(); err != nil {
			return err
		}
	}
	return nil
}

func validateAuditFile(events []domain.AuditEvent, trials map[string]*domain.VigorTrial) error {
	wanted := 0
	digests := map[string]int{}
	for _, trial := range trials {
		for _, event := range trial.Audit {
			wanted++
			digests[event.Digest]++
		}
	}
	if len(events) != wanted {
		return fmt.Errorf("audit file has %d events, snapshot has %d", len(events), wanted)
	}
	for _, event := range events {
		if digests[event.Digest] == 0 {
			return fmt.Errorf("audit file contains unknown digest %s", event.Digest)
		}
		digests[event.Digest]--
	}
	return nil
}
