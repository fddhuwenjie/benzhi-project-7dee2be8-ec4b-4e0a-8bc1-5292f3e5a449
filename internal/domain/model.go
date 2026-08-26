package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type Status string

const (
	Draft            Status = "DRAFT"
	ProtocolLocked   Status = "PROTOCOL_LOCKED"
	ExposureRecorded Status = "EXPOSURE_RECORDED"
	Counting         Status = "COUNTING"
	Reviewed         Status = "REVIEWED"
	Released         Status = "RELEASED"
	Archived         Status = "ARCHIVED"
)

type Protocol struct {
	SampleSize    int       `json:"sample_size"`
	AgeTempC      float64   `json:"aging_temp_c"`
	AgeHumidity   float64   `json:"aging_humidity"`
	JudgementDate string    `json:"judgement_date"`
	Rounds        int       `json:"rounds"`
	Threshold     float64   `json:"threshold"`
	LockedAt      time.Time `json:"locked_at,omitempty"`
}

type Exposure struct {
	GroupCode    string    `json:"group_code"`
	StartedAt    time.Time `json:"started_at"`
	EndedAt      time.Time `json:"ended_at"`
	Device       string    `json:"device"`
	TemperatureC float64   `json:"temperature_c"`
	Humidity     float64   `json:"humidity"`
	Operator     string    `json:"operator"`
	Evidence     string    `json:"evidence"`
}

type GerminationRound struct {
	ID                string    `json:"id"`
	TrialID           string    `json:"trial_id"`
	GroupCode         string    `json:"group_code"`
	RoundNo           int       `json:"round_no"`
	NormalCount       int       `json:"normal_count"`
	AbnormalCount     int       `json:"abnormal_count"`
	UngerminatedCount int       `json:"ungerminated_count"`
	ObservedAt        time.Time `json:"observed_at"`
	Operator          string    `json:"operator"`
	VigorRate         float64   `json:"vigor_rate"`
	EvidenceNote      string    `json:"evidence_note"`
}

type CountCorrection struct {
	GroupCode string           `json:"group_code"`
	RoundNo   int              `json:"round_no"`
	Before    GerminationRound `json:"before"`
	After     GerminationRound `json:"after"`
	Actor     string           `json:"actor"`
	Reason    string           `json:"reason"`
	Evidence  string           `json:"evidence"`
	At        time.Time        `json:"at"`
}

type RemediationVersion struct {
	RootCause   string    `json:"root_cause"`
	Disposition string    `json:"disposition"`
	Evidence    string    `json:"evidence"`
	Actor       string    `json:"actor"`
	At          time.Time `json:"at"`
}

type DeviationDecision struct {
	Decision    string    `json:"decision"`
	RootCause   string    `json:"root_cause"`
	Disposition string    `json:"disposition"`
	Reviewer    string    `json:"reviewer"`
	At          time.Time `json:"at"`
}

type DeviationCase struct {
	ID           string               `json:"id"`
	TrialID      string               `json:"trial_id"`
	Kind         string               `json:"kind"`
	Severity     string               `json:"severity"`
	DetectedAt   time.Time            `json:"detected_at"`
	ReportedBy   string               `json:"reported_by"`
	RootCause    string               `json:"root_cause"`
	Disposition  string               `json:"disposition"`
	Reviewer     string               `json:"reviewer"`
	Decision     string               `json:"decision"`
	ResolvedAt   *time.Time           `json:"resolved_at,omitempty"`
	Stage        string               `json:"stage,omitempty"`
	GroupCode    string               `json:"group_code,omitempty"`
	RoundNo      int                  `json:"round_no,omitempty"`
	Device       string               `json:"device,omitempty"`
	Measured     float64              `json:"measured,omitempty"`
	Planned      float64              `json:"planned,omitempty"`
	LowerBound   float64              `json:"lower_bound,omitempty"`
	UpperBound   float64              `json:"upper_bound,omitempty"`
	Automatic    bool                 `json:"automatic,omitempty"`
	Remediations []RemediationVersion `json:"remediations,omitempty"`
	Decisions    []DeviationDecision  `json:"decisions,omitempty"`
}

type AuditEvent struct {
	At        time.Time      `json:"at"`
	Type      string         `json:"type"`
	Actor     string         `json:"actor"`
	RequestID string         `json:"request_id"`
	Revision  int            `json:"revision"`
	Digest    string         `json:"digest"`
	Data      map[string]any `json:"data,omitempty"`
}

type VigorTrial struct {
	ID              string             `json:"id"`
	SeedLotCode     string             `json:"seed_lot_code"`
	CropName        string             `json:"crop_name"`
	Owner           string             `json:"owner"`
	Status          Status             `json:"status"`
	Protocol        Protocol           `json:"protocol"`
	Groups          []string           `json:"groups"`
	Exposures       []Exposure         `json:"exposures"`
	Rounds          []GerminationRound `json:"rounds"`
	Corrections     []CountCorrection  `json:"count_corrections,omitempty"`
	Deviations      []DeviationCase    `json:"deviations"`
	Audit           []AuditEvent       `json:"audit"`
	Revision        int                `json:"revision"`
	CreatedAt       time.Time          `json:"created_at"`
	ReleasedAt      *time.Time         `json:"released_at,omitempty"`
	ArchiveDigest   string             `json:"archive_digest,omitempty"`
	CountReviewer   string             `json:"count_reviewer,omitempty"`
	ReleaseDecision *ReleaseDecision   `json:"release_decision,omitempty"`
	SealedChecklist *ArchiveChecklist  `json:"sealed_checklist,omitempty"`
}

var ErrConflict = errors.New("revision conflict")
var ErrInvalidTransition = errors.New("invalid status transition")

func (t *VigorTrial) Transition(next Status) error {
	valid := map[Status]map[Status]bool{Draft: {ProtocolLocked: true}, ProtocolLocked: {ExposureRecorded: true}, ExposureRecorded: {Counting: true}, Counting: {Reviewed: true}, Reviewed: {Released: true}, Released: {Archived: true}}
	if !valid[t.Status][next] {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, t.Status, next)
	}
	t.Status = next
	return nil
}

func (t *VigorTrial) ValidateProtocol() error {
	if issues := validateProtocolFields(t.Protocol, t.Groups); len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}

func (t *VigorTrial) AddRound(r GerminationRound) error {
	if !t.HasGroup(r.GroupCode) {
		return errors.New("group_code 不属于批次")
	}
	if r.RoundNo < 1 || r.RoundNo > t.Protocol.Rounds {
		return errors.New("round number outside protocol")
	}
	if r.NormalCount < 0 || r.AbnormalCount < 0 || r.UngerminatedCount < 0 {
		return errors.New("counts cannot be negative")
	}
	if r.NormalCount+r.AbnormalCount+r.UngerminatedCount != t.Protocol.SampleSize {
		return errors.New("counts must equal sample size")
	}
	if r.Operator == "" {
		return errors.New("operator 必填")
	}
	for _, x := range t.Rounds {
		if x.GroupCode == r.GroupCode && x.RoundNo == r.RoundNo {
			return errors.New("round already submitted")
		}
	}
	previous, hasPrevious := t.Round(r.GroupCode, r.RoundNo-1)
	if r.RoundNo > 1 && !hasPrevious {
		return fmt.Errorf("缺少前一轮计数: round_no=%d", r.RoundNo-1)
	}
	if hasPrevious {
		if r.NormalCount < previous.NormalCount {
			return errors.New("normal_count 不得低于前一轮累计值")
		}
		if r.UngerminatedCount > previous.UngerminatedCount {
			return errors.New("ungerminated_count 不得高于前一轮累计值")
		}
	}
	r.VigorRate = float64(r.NormalCount) / float64(t.Protocol.SampleSize) * 100
	t.Rounds = append(t.Rounds, r)
	return nil
}

func (t *VigorTrial) Round(group string, number int) (GerminationRound, bool) {
	for _, round := range t.Rounds {
		if round.GroupCode == group && round.RoundNo == number {
			return round, true
		}
	}
	return GerminationRound{}, false
}

func (t *VigorTrial) CompleteCounting() bool { return len(t.Rounds) >= len(t.Groups)*t.Protocol.Rounds }

func (t *VigorTrial) Gate() (bool, []string) {
	var issues []string
	if !t.CompleteCounting() {
		issues = append(issues, "计数轮次未完成")
	}
	for _, r := range t.Rounds {
		if r.VigorRate < t.Protocol.Threshold {
			issues = append(issues, "活力指标低于阈值: "+r.GroupCode)
		}
	}
	for _, d := range t.Deviations {
		if d.Decision != "ACCEPTED" {
			issues = append(issues, "存在未裁决偏差")
		}
	}
	return len(issues) == 0, issues
}

func Digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
