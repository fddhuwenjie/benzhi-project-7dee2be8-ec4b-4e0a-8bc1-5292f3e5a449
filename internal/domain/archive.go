package domain

import "time"

type IntegrityCheck struct {
	Name          string `json:"name"`
	Passed        bool   `json:"passed"`
	CurrentDigest string `json:"current_digest,omitempty"`
	SealedDigest  string `json:"sealed_digest,omitempty"`
	Message       string `json:"message,omitempty"`
}

type ArchiveProof struct {
	TrialID    string           `json:"trial_id"`
	Revision   int              `json:"revision"`
	Passed     bool             `json:"passed"`
	VerifiedAt time.Time        `json:"verified_at"`
	Checks     []IntegrityCheck `json:"checks"`
}

func (t *VigorTrial) VerifyArchive(at time.Time) ArchiveProof {
	proof := ArchiveProof{TrialID: t.ID, Revision: t.Revision, Passed: true, VerifiedAt: at}
	add := func(check IntegrityCheck) {
		proof.Checks = append(proof.Checks, check)
		proof.Passed = proof.Passed && check.Passed
	}
	if t.SealedChecklist == nil {
		add(IntegrityCheck{Name: "ARCHIVE_MANIFEST_PRESENT", Passed: false, Message: "归档清单摘要缺失"})
		add(IntegrityCheck{Name: "PROTOCOL_DIGEST", Passed: false, CurrentDigest: Digest(t.Protocol), Message: "缺少封存方案摘要"})
		add(IntegrityCheck{Name: "COUNT_DIGEST", Passed: false, CurrentDigest: Digest(t.Rounds), Message: "缺少封存计数摘要"})
		add(IntegrityCheck{Name: "AUDIT_CHAIN", Passed: false, Message: "缺少封存审计链头"})
		add(IntegrityCheck{Name: "ARCHIVE_MANIFEST", Passed: false, CurrentDigest: t.ArchiveDigest, Message: "封存清单摘要缺失"})
		return proof
	}
	sealed := t.SealedChecklist
	protocolDigest := Digest(t.Protocol)
	add(IntegrityCheck{Name: "PROTOCOL_DIGEST", Passed: protocolDigest == sealed.ProtocolDigest, CurrentDigest: protocolDigest, SealedDigest: sealed.ProtocolDigest, Message: mismatchMessage(protocolDigest == sealed.ProtocolDigest, "方案摘要不一致")})
	roundDigest := Digest(t.Rounds)
	add(IntegrityCheck{Name: "COUNT_DIGEST", Passed: roundDigest == sealed.RoundDigest, CurrentDigest: roundDigest, SealedDigest: sealed.RoundDigest, Message: mismatchMessage(roundDigest == sealed.RoundDigest, "计数摘要不一致")})
	head := ""
	if len(t.Audit) > 0 {
		head = t.Audit[len(t.Audit)-1].Digest
	}
	auditOK := t.VerifyIntegrity() == nil && head == sealed.AuditHead
	add(IntegrityCheck{Name: "AUDIT_CHAIN", Passed: auditOK, CurrentDigest: head, SealedDigest: sealed.AuditHead, Message: mismatchMessage(auditOK, "审计链修订不连续、事件摘要或链头不一致")})
	currentManifest := t.Checklist()
	manifestDigest := Digest(currentManifest)
	sealedDigest := Digest(*sealed)
	manifestOK := manifestDigest == sealedDigest && t.ArchiveDigest == sealedDigest
	add(IntegrityCheck{Name: "ARCHIVE_MANIFEST", Passed: manifestOK, CurrentDigest: manifestDigest, SealedDigest: t.ArchiveDigest, Message: mismatchMessage(manifestOK, "封存清单字段或摘要不一致")})
	return proof
}

func mismatchMessage(ok bool, message string) string {
	if ok {
		return ""
	}
	return message
}
