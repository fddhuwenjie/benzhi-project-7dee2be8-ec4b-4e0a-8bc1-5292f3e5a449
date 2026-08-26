package domain

import "time"

type GateReport struct {
	Passed              bool         `json:"passed"`
	Completeness        bool         `json:"completeness"`
	ThresholdSatisfied  bool         `json:"threshold_satisfied"`
	DeviationFinal      bool         `json:"deviation_final"`
	DualReviewSatisfied bool         `json:"dual_review_satisfied"`
	SeparationSatisfied bool         `json:"separation_satisfied"`
	Issues              []string     `json:"issues"`
	Blocks              []GateBlock  `json:"blocks"`
	Participants        Participants `json:"participants"`
}

type Participants struct {
	ExposureOperators  []string `json:"exposure_operators"`
	CountOperators     []string `json:"count_operators"`
	DeviationReporters []string `json:"deviation_reporters"`
	DeviationReviewers []string `json:"deviation_reviewers"`
}

type GateBlock struct {
	Code        string   `json:"code"`
	Message     string   `json:"message"`
	People      []string `json:"people,omitempty"`
	Remediation string   `json:"remediation"`
}

func (r GateReport) Clone() GateReport {
	r.Issues = append([]string(nil), r.Issues...)
	r.Blocks = append([]GateBlock(nil), r.Blocks...)
	for i := range r.Blocks {
		r.Blocks[i].People = append([]string(nil), r.Blocks[i].People...)
	}
	r.Participants.ExposureOperators = append([]string(nil), r.Participants.ExposureOperators...)
	r.Participants.CountOperators = append([]string(nil), r.Participants.CountOperators...)
	r.Participants.DeviationReporters = append([]string(nil), r.Participants.DeviationReporters...)
	r.Participants.DeviationReviewers = append([]string(nil), r.Participants.DeviationReviewers...)
	return r
}

func (t *VigorTrial) GateReport() GateReport { return t.GateReportFor("") }

func (t *VigorTrial) GateReportFor(signer string) GateReport {
	report := GateReport{Completeness: t.CompleteCounting(), ThresholdSatisfied: true, DeviationFinal: true, DualReviewSatisfied: true, SeparationSatisfied: true, Participants: t.Participants()}
	if !report.Completeness {
		report.addBlock("COUNTING_INCOMPLETE", "计数轮次未完成", nil, "补齐所有分组的计划轮次")
	}
	for _, round := range t.Rounds {
		if round.VigorRate < t.Protocol.Threshold {
			if !t.acceptedThresholdDeviation(round.GroupCode, round.RoundNo) {
				report.ThresholdSatisfied = false
				report.addBlock("THRESHOLD_NOT_MET", "活力指标低于阈值: "+round.GroupCode, nil, "完成对应偏差处置并复裁")
			}
		}
	}
	for _, deviation := range t.Deviations {
		if deviation.Decision != "ACCEPTED" {
			report.DeviationFinal = false
			report.DualReviewSatisfied = false
			report.addBlock("DEVIATION_NOT_ACCEPTED", "存在未接受的偏差: "+deviation.ID, []string{deviation.ReportedBy, deviation.Reviewer}, "完成整改并由复核员接受")
		}
	}
	if t.CountReviewer == "" && (t.Status == Reviewed || t.Status == Released || t.Status == Archived) {
		report.DualReviewSatisfied = false
		report.addBlock("COUNT_REVIEW_REQUIRED", "缺少独立计数复核员", nil, "由未参与计数的复核员完成复核")
	}
	if signer != "" {
		if signer == t.Owner {
			report.SeparationSatisfied = false
			report.addBlock("SIGNER_IS_OWNER", "签发人不得为批次负责人", []string{signer}, "改由独立负责人签发")
		}
		for _, reviewer := range report.Participants.DeviationReviewers {
			if reviewer != signer {
				continue
			}
			report.SeparationSatisfied = false
			report.addBlock("SIGNER_REVIEWED_DEVIATION", "签发人不得为偏差复核员", []string{signer}, "改由未参与偏差复核的负责人签发")
			break
		}
	}
	report.Passed = report.Completeness && report.ThresholdSatisfied && report.DeviationFinal && report.DualReviewSatisfied && report.SeparationSatisfied
	return report
}

func (t *VigorTrial) acceptedThresholdDeviation(group string, round int) bool {
	for _, deviation := range t.Deviations {
		if deviation.Kind == "VIGOR_BELOW_THRESHOLD" && deviation.GroupCode == group && deviation.RoundNo == round && deviation.Decision == "ACCEPTED" {
			return true
		}
	}
	return false
}

func (r *GateReport) addBlock(code, message string, people []string, remediation string) {
	r.Issues = append(r.Issues, message)
	r.Blocks = append(r.Blocks, GateBlock{Code: code, Message: message, People: people, Remediation: remediation})
}

type ReleaseDecision struct {
	Signer   string     `json:"signer"`
	Revision int        `json:"revision"`
	At       time.Time  `json:"at"`
	Report   GateReport `json:"report"`
	Digest   string     `json:"digest"`
}

type ArchiveChecklist struct {
	TrialID        string   `json:"trial_id"`
	Revision       int      `json:"revision"`
	SeedLotCode    string   `json:"seed_lot_code"`
	ProtocolDigest string   `json:"protocol_digest"`
	RoundDigest    string   `json:"round_digest"`
	AuditHead      string   `json:"audit_head"`
	Items          []string `json:"items"`
}

func (t *VigorTrial) Checklist() ArchiveChecklist {
	head := ""
	if len(t.Audit) > 0 {
		head = t.Audit[len(t.Audit)-1].Digest
	}
	return ArchiveChecklist{TrialID: t.ID, Revision: t.Revision, SeedLotCode: t.SeedLotCode, ProtocolDigest: Digest(t.Protocol), RoundDigest: Digest(t.Rounds), AuditHead: head, Items: []string{"审计链完整", "双人复核完成", "活力指标达标", "放行决定已签发"}}
}
