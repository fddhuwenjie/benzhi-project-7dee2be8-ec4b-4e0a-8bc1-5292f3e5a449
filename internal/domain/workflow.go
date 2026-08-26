package domain

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const (
	DeviationPendingReview = "PENDING_REVIEW"
	DeviationRemediation   = "REMEDIATION_REQUIRED"
	DeviationPendingRetry  = "PENDING_REREVIEW"
	DeviationAccepted      = "ACCEPTED"
)

type FieldIssue struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationError struct {
	Issues []FieldIssue `json:"issues"`
}

func (e *ValidationError) Error() string {
	if len(e.Issues) == 0 {
		return "参数校验失败"
	}
	return e.Issues[0].Field + ": " + e.Issues[0].Message
}

type ValueRange struct {
	Minimum float64 `json:"minimum"`
	Maximum float64 `json:"maximum"`
}

type ProtocolPreflight struct {
	Summary              string       `json:"summary"`
	ExpectedRevision     int          `json:"expected_revision"`
	GroupCount           int          `json:"group_count"`
	ExpectedCountRecords int          `json:"expected_count_records"`
	TemperatureRange     ValueRange   `json:"temperature_range"`
	HumidityRange        ValueRange   `json:"humidity_range"`
	JudgementBeforeTrial bool         `json:"judgement_before_trial"`
	BlockingIssues       []FieldIssue `json:"blocking_issues"`
}

func (t *VigorTrial) PreflightProtocol(protocol Protocol, expectedRevision int) ProtocolPreflight {
	lowTemp, highTemp := protocol.ExposureTemperatureRange()
	lowHumidity, highHumidity := protocol.ExposureHumidityRange()
	result := ProtocolPreflight{
		ExpectedRevision:     expectedRevision,
		GroupCount:           len(t.Groups),
		ExpectedCountRecords: len(t.Groups) * protocol.Rounds,
		TemperatureRange:     ValueRange{Minimum: lowTemp, Maximum: highTemp},
		HumidityRange:        ValueRange{Minimum: lowHumidity, Maximum: highHumidity},
		BlockingIssues:       validateProtocolFields(protocol, t.Groups),
	}
	if day, err := time.Parse("2006-01-02", protocol.JudgementDate); err == nil {
		createdDay := time.Date(t.CreatedAt.Year(), t.CreatedAt.Month(), t.CreatedAt.Day(), 0, 0, 0, 0, time.UTC)
		result.JudgementBeforeTrial = day.Before(createdDay)
	}
	result.Summary = Digest(struct {
		TrialID          string
		ExpectedRevision int
		Protocol         Protocol
		Groups           []string
	}{t.ID, expectedRevision, protocolForDigest(protocol), append([]string(nil), t.Groups...)})
	return result
}

func protocolForDigest(protocol Protocol) Protocol {
	protocol.LockedAt = time.Time{}
	return protocol
}

func validateProtocolFields(protocol Protocol, groups []string) []FieldIssue {
	var issues []FieldIssue
	if protocol.SampleSize <= 0 || protocol.SampleSize > 100000 {
		issues = append(issues, FieldIssue{Field: "sample_size", Message: "必须在 1 到 100000 之间"})
	}
	if protocol.AgeTempC < -20 || protocol.AgeTempC > 80 {
		issues = append(issues, FieldIssue{Field: "aging_temp_c", Message: "必须在 -20 到 80 之间"})
	}
	if protocol.AgeHumidity < 0 || protocol.AgeHumidity > 100 {
		issues = append(issues, FieldIssue{Field: "aging_humidity", Message: "必须在 0 到 100 之间"})
	}
	if protocol.Rounds < 1 || protocol.Rounds > 20 {
		issues = append(issues, FieldIssue{Field: "rounds", Message: "必须在 1 到 20 之间"})
	}
	if _, err := time.Parse("2006-01-02", protocol.JudgementDate); err != nil {
		issues = append(issues, FieldIssue{Field: "judgement_date", Message: "必须为 YYYY-MM-DD"})
	}
	if protocol.Threshold < 0 || protocol.Threshold > 100 {
		issues = append(issues, FieldIssue{Field: "threshold", Message: "必须在 0 到 100 之间"})
	}
	if len(groups) == 0 {
		issues = append(issues, FieldIssue{Field: "groups", Message: "至少需要一个分组"})
	}
	return issues
}

type ExposureDeviation struct {
	Kind       string
	Severity   string
	Measured   float64
	Planned    float64
	LowerBound float64
	UpperBound float64
}

func (p Protocol) ExposureDeviations(exposure Exposure) []ExposureDeviation {
	minTemp, maxTemp := p.ExposureTemperatureRange()
	minHumidity, maxHumidity := p.ExposureHumidityRange()
	var deviations []ExposureDeviation
	if exposure.TemperatureC < minTemp {
		deviations = append(deviations, exposureDeviation("TEMPERATURE_LOW", exposure.TemperatureC, p.AgeTempC, minTemp, maxTemp, minTemp-exposure.TemperatureC, 1))
	}
	if exposure.TemperatureC > maxTemp {
		deviations = append(deviations, exposureDeviation("TEMPERATURE_HIGH", exposure.TemperatureC, p.AgeTempC, minTemp, maxTemp, exposure.TemperatureC-maxTemp, 1))
	}
	if exposure.Humidity < minHumidity {
		deviations = append(deviations, exposureDeviation("HUMIDITY_LOW", exposure.Humidity, p.AgeHumidity, minHumidity, maxHumidity, minHumidity-exposure.Humidity, 5))
	}
	if exposure.Humidity > maxHumidity {
		deviations = append(deviations, exposureDeviation("HUMIDITY_HIGH", exposure.Humidity, p.AgeHumidity, minHumidity, maxHumidity, exposure.Humidity-maxHumidity, 5))
	}
	return deviations
}

func exposureDeviation(kind string, measured, planned, lower, upper, excess, tolerance float64) ExposureDeviation {
	ratio := excess / tolerance
	severity := "LOW"
	if ratio > 2 {
		severity = "HIGH"
	} else if ratio > 1 {
		severity = "MEDIUM"
	}
	return ExposureDeviation{Kind: kind, Severity: severity, Measured: measured, Planned: planned, LowerBound: lower, UpperBound: upper}
}

func NewExposureDeviation(trialID string, exposure Exposure, issue ExposureDeviation, reporter string) DeviationCase {
	return DeviationCase{
		ID: fmt.Sprintf("AUTO-EXPOSURE-%s-%s", exposure.GroupCode, issue.Kind), TrialID: trialID,
		Kind: issue.Kind, Severity: issue.Severity, DetectedAt: time.Now().UTC(), ReportedBy: reporter,
		RootCause: "设备读数超出方案容差", Stage: DeviationPendingReview, GroupCode: exposure.GroupCode,
		Device: exposure.Device, Measured: issue.Measured, Planned: issue.Planned,
		LowerBound: issue.LowerBound, UpperBound: issue.UpperBound, Automatic: true,
	}
}

func (t *VigorTrial) ValidateRoundAt(round GerminationRound, observedAt time.Time) error {
	exposure, found := t.ExposureFor(round.GroupCode)
	if !found {
		return errors.New("该分组尚未登记老化条件")
	}
	if !observedAt.After(exposure.EndedAt) {
		return errors.New("observed_at 必须晚于该分组老化结束时间")
	}
	return nil
}

func (t *VigorTrial) ExposureFor(group string) (Exposure, bool) {
	for _, exposure := range t.Exposures {
		if exposure.GroupCode == group {
			return exposure, true
		}
	}
	return Exposure{}, false
}

func (t *VigorTrial) CorrectRound(group string, number, normal, abnormal, ungerminated int, actor, reason, evidence string, at time.Time) error {
	if strings.TrimSpace(actor) == "" || strings.TrimSpace(reason) == "" || strings.TrimSpace(evidence) == "" {
		return errors.New("更正人、原因和证据说明必填")
	}
	index := -1
	for i := range t.Rounds {
		if t.Rounds[i].GroupCode == group && t.Rounds[i].RoundNo == number {
			index = i
			break
		}
	}
	if index < 0 {
		return errors.New("计数记录不存在")
	}
	before := t.Rounds[index]
	after := before
	after.NormalCount, after.AbnormalCount, after.UngerminatedCount = normal, abnormal, ungerminated
	after.Operator, after.ObservedAt, after.EvidenceNote = actor, at, evidence
	if err := t.validateCountValues(after); err != nil {
		return err
	}
	if previous, ok := t.Round(group, number-1); ok {
		if after.NormalCount < previous.NormalCount {
			return errors.New("normal_count 不得低于前一轮累计值")
		}
		if after.UngerminatedCount > previous.UngerminatedCount {
			return errors.New("ungerminated_count 不得高于前一轮累计值")
		}
	}
	if next, ok := t.Round(group, number+1); ok {
		if after.NormalCount > next.NormalCount {
			return errors.New("normal_count 不得高于下一轮累计值")
		}
		if after.UngerminatedCount < next.UngerminatedCount {
			return errors.New("ungerminated_count 不得低于下一轮累计值")
		}
	}
	after.VigorRate = float64(after.NormalCount) / float64(t.Protocol.SampleSize) * 100
	t.Rounds[index] = after
	t.Corrections = append(t.Corrections, CountCorrection{GroupCode: group, RoundNo: number, Before: before, After: after, Actor: actor, Reason: reason, Evidence: evidence, At: at})
	return nil
}

func (t *VigorTrial) validateCountValues(round GerminationRound) error {
	if round.NormalCount < 0 || round.AbnormalCount < 0 || round.UngerminatedCount < 0 {
		return errors.New("计数不得为负数")
	}
	if round.NormalCount+round.AbnormalCount+round.UngerminatedCount != t.Protocol.SampleSize {
		return errors.New("三类计数之和必须等于锁定样本量")
	}
	return nil
}

func (t *VigorTrial) ReconcileThresholdDeviations(reporter string) {
	wanted := map[string]GerminationRound{}
	for _, round := range t.Rounds {
		if round.VigorRate < t.Protocol.Threshold {
			wanted[RoundKey(round.GroupCode, round.RoundNo)] = round
		}
	}
	kept := make([]DeviationCase, 0, len(t.Deviations)+len(wanted))
	for _, deviation := range t.Deviations {
		if deviation.Automatic && deviation.Kind == "VIGOR_BELOW_THRESHOLD" {
			key := RoundKey(deviation.GroupCode, deviation.RoundNo)
			if _, ok := wanted[key]; !ok {
				continue
			}
			delete(wanted, key)
		}
		kept = append(kept, deviation)
	}
	keys := make([]string, 0, len(wanted))
	for key := range wanted {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		round := wanted[key]
		owner := round.Operator
		if owner == "" {
			owner = reporter
		}
		kept = append(kept, DeviationCase{ID: "AUTO-COUNT-" + key, TrialID: t.ID, Kind: "VIGOR_BELOW_THRESHOLD", Severity: thresholdSeverity(t.Protocol.Threshold - round.VigorRate), DetectedAt: time.Now().UTC(), ReportedBy: owner, RootCause: "活力率低于方案阈值", Stage: DeviationPendingReview, GroupCode: round.GroupCode, RoundNo: round.RoundNo, Measured: round.VigorRate, Planned: t.Protocol.Threshold, Automatic: true})
	}
	t.Deviations = kept
}

func thresholdSeverity(delta float64) string {
	if delta > 20 {
		return "HIGH"
	}
	if delta > 10 {
		return "MEDIUM"
	}
	return "LOW"
}

func (t *VigorTrial) Reviewers() []string {
	seen := map[string]bool{}
	for _, deviation := range t.Deviations {
		if deviation.Reviewer != "" {
			seen[deviation.Reviewer] = true
		}
		for _, decision := range deviation.Decisions {
			if decision.Reviewer != "" {
				seen[decision.Reviewer] = true
			}
		}
	}
	result := make([]string, 0, len(seen))
	for person := range seen {
		result = append(result, person)
	}
	sort.Strings(result)
	return result
}

func (t *VigorTrial) Participants() Participants {
	exposures, reporters := map[string]bool{}, map[string]bool{}
	for _, exposure := range t.Exposures {
		if exposure.Operator != "" {
			exposures[exposure.Operator] = true
		}
	}
	for _, deviation := range t.Deviations {
		if deviation.ReportedBy != "" {
			reporters[deviation.ReportedBy] = true
		}
	}
	return Participants{ExposureOperators: sortedPeople(exposures), CountOperators: t.CountOperators(), DeviationReporters: sortedPeople(reporters), DeviationReviewers: t.Reviewers()}
}

func sortedPeople(seen map[string]bool) []string {
	result := make([]string, 0, len(seen))
	for person := range seen {
		result = append(result, person)
	}
	sort.Strings(result)
	return result
}

func (t *VigorTrial) CountOperators() []string {
	seen := map[string]bool{}
	for _, round := range t.Rounds {
		seen[round.Operator] = true
	}
	result := make([]string, 0, len(seen))
	for person := range seen {
		result = append(result, person)
	}
	sort.Strings(result)
	return result
}

func (t *VigorTrial) IsCountOperator(person string) bool {
	for _, operator := range t.CountOperators() {
		if operator == person {
			return true
		}
	}
	return false
}

func floatEqual(a, b float64) bool { return math.Abs(a-b) < 0.000001 }
