package domain

import "fmt"

func (t *VigorTrial) VerifyIntegrity() error {
	if t.Revision != len(t.Audit) {
		return fmt.Errorf("trial %s revision %d does not match %d audit events", t.ID, t.Revision, len(t.Audit))
	}
	previous := ""
	for index, event := range t.Audit {
		wantRevision := index + 1
		if event.Revision != wantRevision {
			return fmt.Errorf("trial %s audit revision discontinuity at %d", t.ID, wantRevision)
		}
		recorded := event.Digest
		calculated := auditEventDigest(previous, event)
		if recorded != calculated {
			return fmt.Errorf("trial %s audit digest mismatch at revision %d", t.ID, wantRevision)
		}
		previous = recorded
	}
	if t.Status == Archived && t.ArchiveDigest == "" {
		return fmt.Errorf("trial %s archived without digest", t.ID)
	}
	return nil
}

func (t *VigorTrial) Clone() *VigorTrial {
	copy := *t
	copy.Groups = append([]string(nil), t.Groups...)
	copy.Exposures = append([]Exposure(nil), t.Exposures...)
	copy.Rounds = append([]GerminationRound(nil), t.Rounds...)
	copy.Corrections = append([]CountCorrection(nil), t.Corrections...)
	copy.Deviations = make([]DeviationCase, len(t.Deviations))
	for i := range t.Deviations {
		copy.Deviations[i] = t.Deviations[i]
	}
	copy.Audit = make([]AuditEvent, len(t.Audit))
	for i := range t.Audit {
		copy.Audit[i] = t.Audit[i]
		if t.Audit[i].Data != nil {
			copy.Audit[i].Data = make(map[string]any, len(t.Audit[i].Data))
			for key, value := range t.Audit[i].Data {
				copy.Audit[i].Data[key] = value
			}
		}
	}
	if t.ReleaseDecision != nil {
		decision := *t.ReleaseDecision
		decision.Report = t.ReleaseDecision.Report.Clone()
		copy.ReleaseDecision = &decision
	}
	if t.SealedChecklist != nil {
		checklist := *t.SealedChecklist
		checklist.Items = append([]string(nil), t.SealedChecklist.Items...)
		copy.SealedChecklist = &checklist
	}
	return &copy
}
