package application

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"seed-vigor-gate/internal/domain"
	"seed-vigor-gate/internal/store"
)

type Service struct {
	st      *store.Store
	mu      sync.Mutex
	results map[string]*domain.VigorTrial
	queries map[TrialFilter]TrialPage
}

const (
	RoleTechnician = "TECHNICIAN"
	RoleReviewer   = "REVIEWER"
	RoleLead       = "LEAD"
)

func authorize(actor, role string, allowed ...string) error {
	if actor == "" {
		return errors.New("actor 必填")
	}
	for _, candidate := range allowed {
		if role == candidate {
			return nil
		}
	}
	return fmt.Errorf("角色 %s 无权执行该操作", role)
}

func New(st *store.Store) *Service {
	return &Service{
		st:      st,
		results: map[string]*domain.VigorTrial{},
		queries: map[TrialFilter]TrialPage{},
	}
}
func (s *Service) get(id string) (*domain.VigorTrial, error) { return s.st.Get(id) }
func (s *Service) replay(requestID string) (*domain.VigorTrial, bool) {
	if requestID == "" {
		return nil, false
	}
	if t, ok := s.results[requestID]; ok {
		return t, true
	}
	for _, t := range s.st.List() {
		for _, event := range t.Audit {
			if event.RequestID == requestID {
				s.results[requestID] = t
				return t, true
			}
		}
	}
	return nil, false
}
func (s *Service) commit(t *domain.VigorTrial, actor, req, typ string, data map[string]any) (*domain.VigorTrial, error) {
	if req == "" {
		return nil, errors.New("request_id 必填")
	}
	t.Revision++
	t.Audit = append(t.Audit, domain.NewAudit(t, typ, actor, req, data))
	if err := s.st.Save(t, req); err != nil {
		return nil, err
	}
	s.results[req] = t
	s.queries = map[TrialFilter]TrialPage{}
	return t, nil
}
func (s *Service) check(t *domain.VigorTrial, rev int) error {
	if rev >= 0 && t.Revision != rev {
		return domain.ErrConflict
	}
	return nil
}
func (s *Service) Create(seed, crop, owner string, groups []string, actor, role, req string) (*domain.VigorTrial, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if x, ok := s.replay(req); ok {
		return x, nil
	}
	if err := authorize(actor, role, RoleTechnician); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	t := &domain.VigorTrial{ID: fmt.Sprintf("TR-%d", now.UnixNano()), SeedLotCode: seed, CropName: crop, Owner: owner, Groups: groups, Status: domain.Draft, Revision: 0, CreatedAt: now}
	if err := t.ValidateIdentity(); err != nil {
		return nil, err
	}
	return s.commit(t, actor, req, "TRIAL_CREATED", map[string]any{"seed_lot_code": seed})
}
func (s *Service) LockProtocol(id string, p domain.Protocol, rev int, actor, role, req string) (*domain.VigorTrial, error) {
	preflight, err := s.PreflightProtocol(id, p, rev)
	if err != nil {
		return nil, err
	}
	return s.LockProtocolConfirmed(id, p, rev, preflight.Summary, actor, role, req)
}

func (s *Service) PreflightProtocol(id string, p domain.Protocol, rev int) (domain.ProtocolPreflight, error) {
	t, err := s.get(id)
	if err != nil {
		return domain.ProtocolPreflight{}, err
	}
	if err = s.check(t, rev); err != nil {
		return domain.ProtocolPreflight{}, err
	}
	if t.Status != domain.Draft {
		return domain.ProtocolPreflight{}, domain.ErrInvalidTransition
	}
	result := t.PreflightProtocol(p, rev)
	return result, nil
}

func (s *Service) LockProtocolConfirmed(id string, p domain.Protocol, rev int, summary, actor, role, req string) (*domain.VigorTrial, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if x, ok := s.replay(req); ok {
		return x, nil
	}
	if err := authorize(actor, role, RoleLead); err != nil {
		return nil, err
	}
	t, e := s.get(id)
	if e != nil {
		return nil, e
	}
	if e = s.check(t, rev); e != nil {
		return nil, e
	}
	if t.Status != domain.Draft {
		return nil, domain.ErrInvalidTransition
	}
	preflight := t.PreflightProtocol(p, rev)
	if len(preflight.BlockingIssues) > 0 {
		return nil, &domain.ValidationError{Issues: preflight.BlockingIssues}
	}
	if summary == "" {
		return nil, errors.New("preflight_summary 必填")
	}
	if summary != preflight.Summary {
		return nil, errors.New("预检摘要与方案内容不一致")
	}
	t.Protocol = p
	if e = t.ValidateProtocol(); e != nil {
		return nil, e
	}
	t.Protocol.LockedAt = time.Now().UTC()
	if e = t.Transition(domain.ProtocolLocked); e != nil {
		return nil, e
	}
	return s.commit(t, actor, req, "PROTOCOL_LOCKED", map[string]any{"preflight_summary": summary})
}
func (s *Service) RecordExposure(id string, x domain.Exposure, rev int, actor, role, req string) (*domain.VigorTrial, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if saved, ok := s.replay(req); ok {
		return saved, nil
	}
	if err := authorize(actor, role, RoleTechnician); err != nil {
		return nil, err
	}
	t, e := s.get(id)
	if e != nil {
		return nil, e
	}
	if e = s.check(t, rev); e != nil {
		return nil, e
	}
	if t.Status != domain.ProtocolLocked {
		return nil, domain.ErrInvalidTransition
	}
	if err := t.ValidateExposure(x); err != nil {
		return nil, err
	}
	for _, saved := range t.Exposures {
		if saved.GroupCode == x.GroupCode {
			return nil, errors.New("该分组已登记老化条件")
		}
	}
	t.Exposures = append(t.Exposures, x)
	for _, issue := range t.Protocol.ExposureDeviations(x) {
		deviation := domain.NewExposureDeviation(t.ID, x, issue, actor)
		duplicate := false
		for _, saved := range t.Deviations {
			if saved.ID == deviation.ID {
				duplicate = true
				break
			}
		}
		if !duplicate {
			t.Deviations = append(t.Deviations, deviation)
		}
	}
	if len(t.Exposures) == len(t.Groups) {
		if e = t.Transition(domain.ExposureRecorded); e != nil {
			return nil, e
		}
	}
	return s.commit(t, actor, req, "EXPOSURE_RECORDED", map[string]any{"group": x.GroupCode})
}
func (s *Service) SubmitRound(id string, r domain.GerminationRound, rev int, actor, role, req string) (*domain.VigorTrial, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if x, ok := s.replay(req); ok {
		return x, nil
	}
	if err := authorize(actor, role, RoleTechnician); err != nil {
		return nil, err
	}
	t, e := s.get(id)
	if e != nil {
		return nil, e
	}
	if e = s.check(t, rev); e != nil {
		return nil, e
	}
	if t.Status != domain.ExposureRecorded && t.Status != domain.Counting {
		return nil, domain.ErrInvalidTransition
	}
	r.TrialID = id
	r.ID = fmt.Sprintf("%s-%d-%d", id, r.RoundNo, len(t.Rounds))
	r.ObservedAt = time.Now().UTC()
	if e = t.ValidateRoundAt(r, r.ObservedAt); e != nil {
		return nil, e
	}
	if e = t.AddRound(r); e != nil {
		return nil, e
	}
	if t.Status == domain.ExposureRecorded {
		if e = t.Transition(domain.Counting); e != nil {
			return nil, e
		}
	}
	return s.commit(t, actor, req, "ROUND_SUBMITTED", map[string]any{"group": r.GroupCode, "round": r.RoundNo})
}

func (s *Service) CorrectRound(id, group string, roundNo, normal, abnormal, ungerminated int, reason, evidence string, rev int, actor, role, req string) (*domain.VigorTrial, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if x, ok := s.replay(req); ok {
		return x, nil
	}
	if err := authorize(actor, role, RoleTechnician); err != nil {
		return nil, err
	}
	t, err := s.get(id)
	if err != nil {
		return nil, err
	}
	if err = s.check(t, rev); err != nil {
		return nil, err
	}
	if t.Status != domain.Counting {
		return nil, domain.ErrInvalidTransition
	}
	if err = t.CorrectRound(group, roundNo, normal, abnormal, ungerminated, actor, reason, evidence, time.Now().UTC()); err != nil {
		return nil, err
	}
	t.ReconcileThresholdDeviations(actor)
	correction := t.Corrections[len(t.Corrections)-1]
	return s.commit(t, actor, req, "ROUND_CORRECTED", map[string]any{"group": group, "round": roundNo, "reason": reason, "evidence": evidence, "before": correction.Before, "after": correction.After})
}
func (s *Service) ReportDeviation(id string, d domain.DeviationCase, rev int, actor, role, req string) (*domain.VigorTrial, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if x, ok := s.replay(req); ok {
		return x, nil
	}
	if err := authorize(actor, role, RoleTechnician); err != nil {
		return nil, err
	}
	t, e := s.get(id)
	if e != nil {
		return nil, e
	}
	if e = s.check(t, rev); e != nil {
		return nil, e
	}
	if t.Status != domain.Counting {
		return nil, domain.ErrInvalidTransition
	}
	d.ID = fmt.Sprintf("DEV-%d", time.Now().UnixNano())
	d.TrialID = id
	d.DetectedAt = time.Now().UTC()
	d.ReportedBy = actor
	d.Stage = domain.DeviationPendingReview
	if err := d.ValidateForReport(); err != nil {
		return nil, err
	}
	t.Deviations = append(t.Deviations, d)
	return s.commit(t, actor, req, "DEVIATION_REPORTED", nil)
}
func (s *Service) ReviewCounts(id, reviewer string, rev int, actor, role, req string) (*domain.VigorTrial, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if x, ok := s.replay(req); ok {
		return x, nil
	}
	if err := authorize(actor, role, RoleReviewer); err != nil {
		return nil, err
	}
	t, e := s.get(id)
	if e != nil {
		return nil, e
	}
	if e = s.check(t, rev); e != nil {
		return nil, e
	}
	if t.Status != domain.Counting {
		return nil, domain.ErrInvalidTransition
	}
	if !t.CompleteCounting() {
		return nil, errors.New("计数轮次未完成")
	}
	if reviewer == "" || reviewer != actor {
		return nil, errors.New("复核员身份不一致")
	}
	if t.IsCountOperator(reviewer) {
		return nil, fmt.Errorf("职责冲突: 复核员 %s 提交过本批次计数", reviewer)
	}
	t.CountReviewer = reviewer
	t.ReconcileThresholdDeviations("")
	if len(t.Deviations) == 0 {
		if e = t.Transition(domain.Reviewed); e != nil {
			return nil, e
		}
	}
	return s.commit(t, reviewer, req, "COUNTS_REVIEWED", map[string]any{"deviations": len(t.Deviations)})
}
func (s *Service) ReviewDeviation(id, devID, decision, rootCause, disposition, reviewer string, rev int, actor, role, req string) (*domain.VigorTrial, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if x, ok := s.replay(req); ok {
		return x, nil
	}
	if err := authorize(actor, role, RoleReviewer); err != nil {
		return nil, err
	}
	t, e := s.get(id)
	if e != nil {
		return nil, e
	}
	if e = s.check(t, rev); e != nil {
		return nil, e
	}
	if t.Status != domain.Counting {
		return nil, domain.ErrInvalidTransition
	}
	if reviewer == "" || reviewer != actor {
		return nil, errors.New("复核员身份不一致")
	}
	if decision != "ACCEPTED" && decision != "REJECTED" {
		return nil, errors.New("decision 必须为 ACCEPTED 或 REJECTED")
	}
	if rootCause == "" || disposition == "" {
		return nil, errors.New("root_cause 和 disposition 必填")
	}
	found := false
	for i := range t.Deviations {
		if t.Deviations[i].ID == devID {
			if t.Deviations[i].Stage != domain.DeviationPendingReview && t.Deviations[i].Stage != domain.DeviationPendingRetry && t.Deviations[i].Stage != "" {
				return nil, errors.New("偏差当前阶段不可裁决")
			}
			now := time.Now().UTC()
			t.Deviations[i].Reviewer = reviewer
			t.Deviations[i].Decision = decision
			t.Deviations[i].RootCause = rootCause
			t.Deviations[i].Disposition = disposition
			t.Deviations[i].Decisions = append(t.Deviations[i].Decisions, domain.DeviationDecision{Decision: decision, RootCause: rootCause, Disposition: disposition, Reviewer: reviewer, At: now})
			if decision == "ACCEPTED" {
				t.Deviations[i].Stage = domain.DeviationAccepted
				t.Deviations[i].ResolvedAt = &now
			} else {
				t.Deviations[i].Stage = domain.DeviationRemediation
				t.Deviations[i].ResolvedAt = nil
			}
			found = true
		}
	}
	if !found {
		return nil, errors.New("deviation not found")
	}
	allResolved := true
	for _, deviation := range t.Deviations {
		if deviation.Decision != "ACCEPTED" {
			allResolved = false
		}
	}
	if allResolved && t.CompleteCounting() && t.Status == domain.Counting {
		if e = t.Transition(domain.Reviewed); e != nil {
			return nil, e
		}
	}
	return s.commit(t, actor, req, "DEVIATION_REVIEWED", map[string]any{"decision": decision})
}

func (s *Service) RemediateDeviation(id, devID, rootCause, disposition, evidence string, rev int, actor, role, req string) (*domain.VigorTrial, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if x, ok := s.replay(req); ok {
		return x, nil
	}
	if err := authorize(actor, role, RoleTechnician); err != nil {
		return nil, err
	}
	t, err := s.get(id)
	if err != nil {
		return nil, err
	}
	if err = s.check(t, rev); err != nil {
		return nil, err
	}
	if t.Status != domain.Counting {
		return nil, domain.ErrInvalidTransition
	}
	if strings.TrimSpace(rootCause) == "" || strings.TrimSpace(disposition) == "" || strings.TrimSpace(evidence) == "" {
		return nil, errors.New("root_cause、disposition 和 evidence 必填")
	}
	found := false
	for i := range t.Deviations {
		deviation := &t.Deviations[i]
		if deviation.ID != devID {
			continue
		}
		if deviation.Stage != domain.DeviationRemediation || deviation.Decision != "REJECTED" {
			return nil, errors.New("偏差尚未被退回整改")
		}
		if deviation.ReportedBy != actor {
			return nil, errors.New("仅原报告技术员可提交整改")
		}
		now := time.Now().UTC()
		deviation.Remediations = append(deviation.Remediations, domain.RemediationVersion{RootCause: rootCause, Disposition: disposition, Evidence: evidence, Actor: actor, At: now})
		deviation.RootCause, deviation.Disposition = rootCause, disposition
		deviation.Decision, deviation.Reviewer = "", ""
		deviation.Stage = domain.DeviationPendingRetry
		found = true
		break
	}
	if !found {
		return nil, errors.New("deviation not found")
	}
	return s.commit(t, actor, req, "DEVIATION_REMEDIATED", map[string]any{"deviation_id": devID, "evidence": evidence})
}
func (s *Service) Release(id string, rev int, actor, role, req string) (*domain.VigorTrial, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if x, ok := s.replay(req); ok {
		return x, nil
	}
	if err := authorize(actor, role, RoleLead); err != nil {
		return nil, err
	}
	t, e := s.get(id)
	if e != nil {
		return nil, e
	}
	if e = s.check(t, rev); e != nil {
		return nil, e
	}
	if t.Status != domain.Reviewed {
		return nil, domain.ErrInvalidTransition
	}
	report := t.GateReportFor(actor)
	if !report.Passed {
		return nil, fmt.Errorf("质量门禁未通过: %v", report.Issues)
	}
	now := time.Now().UTC()
	decision := &domain.ReleaseDecision{Signer: actor, Revision: t.Revision, At: now, Report: report}
	decision.Digest = domain.Digest(struct {
		TrialID  string
		Revision int
		Signer   string
		Report   domain.GateReport
	}{t.ID, t.Revision, actor, report})
	t.ReleaseDecision = decision
	if e = t.Transition(domain.Released); e != nil {
		return nil, e
	}
	t.ReleasedAt = &now
	return s.commit(t, actor, req, "TRIAL_RELEASED", map[string]any{"decision_digest": decision.Digest})
}
func (s *Service) Archive(id string, rev int, actor, role, req string) (*domain.VigorTrial, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if x, ok := s.replay(req); ok {
		return x, nil
	}
	if err := authorize(actor, role, RoleLead); err != nil {
		return nil, err
	}
	t, e := s.get(id)
	if e != nil {
		return nil, e
	}
	if e = s.check(t, rev); e != nil {
		return nil, e
	}
	if t.Status != domain.Released {
		return nil, domain.ErrInvalidTransition
	}
	if e = t.Transition(domain.Archived); e != nil {
		return nil, e
	}
	// archive_digest 不参与本事件链摘要，避免封存清单与事件链头形成摘要循环。
	t.Revision++
	event := domain.NewAudit(t, "TRIAL_ARCHIVED", actor, req, map[string]any{"archive_digest": ""})
	t.Audit = append(t.Audit, event)
	checklist := t.Checklist()
	t.SealedChecklist = &checklist
	t.ArchiveDigest = domain.Digest(checklist)
	t.Audit[len(t.Audit)-1].Data = map[string]any{"archive_digest": t.ArchiveDigest}
	if err := s.st.Save(t, req); err != nil {
		return nil, err
	}
	s.results[req] = t
	s.queries = map[TrialFilter]TrialPage{}
	return t, nil
}
func (s *Service) List() []*domain.VigorTrial                      { return s.st.List() }
func (s *Service) Get(id string) (*domain.VigorTrial, error)       { return s.st.Get(id) }
func (s *Service) Timeline(id string) ([]domain.AuditEvent, error) { return s.st.Timeline(id) }
