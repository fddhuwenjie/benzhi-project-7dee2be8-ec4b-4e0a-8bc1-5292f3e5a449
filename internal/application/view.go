package application

import "seed-vigor-gate/internal/domain"

type TrialView struct {
	Trial                  *domain.VigorTrial   `json:"trial"`
	Gate                   domain.GateReport    `json:"gate"`
	Progress               domain.TrialProgress `json:"progress"`
	RecordedExposureGroups []string             `json:"recorded_exposure_groups"`
	PendingExposureGroups  []string             `json:"pending_exposure_groups"`
	NextAction             string               `json:"next_action"`
	ReadOnly               bool                 `json:"read_only"`
}

func (s *Service) View(id string) (TrialView, error) {
	trial, err := s.Get(id)
	if err != nil {
		return TrialView{}, err
	}
	next := map[domain.Status]string{domain.Draft: "锁定试验方案", domain.ProtocolLocked: "登记全部分组老化条件", domain.ExposureRecorded: "提交萌发计数", domain.Counting: "完成计数与偏差裁决", domain.Reviewed: "执行质量门禁并签发", domain.Released: "签署封存", domain.Archived: "查看只读审计时间线"}[trial.Status]
	recorded := make([]string, 0, len(trial.Exposures))
	pending := make([]string, 0, len(trial.Groups))
	for _, group := range trial.Groups {
		if _, ok := trial.ExposureFor(group); ok {
			recorded = append(recorded, group)
		} else {
			pending = append(pending, group)
		}
	}
	return TrialView{Trial: trial, Gate: trial.GateReport(), Progress: trial.Progress(), RecordedExposureGroups: recorded, PendingExposureGroups: pending, NextAction: next, ReadOnly: trial.Status == domain.Archived}, nil
}

func (s *Service) GatePreview(id, signer string) (domain.GateReport, error) {
	trial, err := s.Get(id)
	if err != nil {
		return domain.GateReport{}, err
	}
	if trial.Status != domain.Reviewed {
		return domain.GateReport{}, domain.ErrInvalidTransition
	}
	return trial.GateReportFor(signer), nil
}
