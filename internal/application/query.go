package application

import (
	"errors"
	"sort"
	"strings"
	"time"

	"seed-vigor-gate/internal/domain"
)

const MaxPageLimit = 100

type TrialFilter struct {
	SeedLotCode string
	CropName    string
	Status      domain.Status
	Owner       string
	Offset      int
	Limit       int
}

type TrialSummary struct {
	ByStatus       map[domain.Status]int `json:"by_status"`
	MissingRounds  int                   `json:"missing_rounds"`
	OpenDeviations int                   `json:"open_deviations"`
}

type TrialPage struct {
	Trials  []*domain.VigorTrial `json:"trials"`
	Total   int                  `json:"total"`
	Offset  int                  `json:"offset"`
	Limit   int                  `json:"limit"`
	Summary TrialSummary         `json:"summary"`
}

func (s *Service) Query(filter TrialFilter) (TrialPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if filter.Offset < 0 {
		return TrialPage{}, errors.New("offset 不能为负数")
	}
	if filter.Limit < 1 || filter.Limit > MaxPageLimit {
		return TrialPage{}, errors.New("limit 必须在 1 到 100 之间")
	}
	if page, ok := s.queries[filter]; ok {
		return page, nil
	}
	result := make([]*domain.VigorTrial, 0)
	summary := TrialSummary{ByStatus: map[domain.Status]int{}}
	for _, trial := range s.List() {
		if filter.SeedLotCode != "" && !containsFold(trial.SeedLotCode, filter.SeedLotCode) {
			continue
		}
		if filter.CropName != "" && !containsFold(trial.CropName, filter.CropName) {
			continue
		}
		if filter.Status != "" && trial.Status != filter.Status {
			continue
		}
		if filter.Owner != "" && !containsFold(trial.Owner, filter.Owner) {
			continue
		}
		result = append(result, trial)
		summary.ByStatus[trial.Status]++
		progress := trial.Progress()
		for _, group := range progress.Groups {
			summary.MissingRounds += len(group.MissingRounds)
		}
		summary.OpenDeviations += progress.OpenDeviations
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID > result[j].ID
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	page := TrialPage{Total: len(result), Offset: filter.Offset, Limit: filter.Limit, Summary: summary}
	if filter.Offset < len(result) {
		end := filter.Offset + filter.Limit
		if end > len(result) {
			end = len(result)
		}
		page.Trials = result[filter.Offset:end]
	}
	s.queries[filter] = page
	return page, nil
}

func containsFold(value, query string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(strings.TrimSpace(query)))
}

type AuditFilter struct {
	Type   string
	Actor  string
	From   time.Time
	To     time.Time
	Offset int
	Limit  int
}

type AuditPage struct {
	Events []domain.AuditEvent `json:"events"`
	Total  int                 `json:"total"`
	Offset int                 `json:"offset"`
	Limit  int                 `json:"limit"`
}

func (s *Service) QueryTimeline(id string, filter AuditFilter) (AuditPage, error) {
	if filter.Offset < 0 {
		return AuditPage{}, errors.New("offset 不能为负数")
	}
	if filter.Limit < 1 || filter.Limit > 200 {
		return AuditPage{}, errors.New("limit 必须在 1 到 200 之间")
	}
	if !filter.From.IsZero() && !filter.To.IsZero() && filter.From.After(filter.To) {
		return AuditPage{}, errors.New("from 不能晚于 to")
	}
	events, err := s.Timeline(id)
	if err != nil {
		return AuditPage{}, err
	}
	filtered := make([]domain.AuditEvent, 0, len(events))
	for _, event := range events {
		if filter.Type != "" && event.Type != filter.Type {
			continue
		}
		if filter.Actor != "" && !containsFold(event.Actor, filter.Actor) {
			continue
		}
		if !filter.From.IsZero() && event.At.Before(filter.From) {
			continue
		}
		if !filter.To.IsZero() && event.At.After(filter.To) {
			continue
		}
		filtered = append(filtered, event)
	}
	sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].At.After(filtered[j].At) })
	page := AuditPage{Total: len(filtered), Offset: filter.Offset, Limit: filter.Limit}
	if filter.Offset < len(filtered) {
		end := filter.Offset + filter.Limit
		if end > len(filtered) {
			end = len(filtered)
		}
		page.Events = filtered[filter.Offset:end]
	}
	return page, nil
}

func (s *Service) ArchiveProof(id string) (domain.ArchiveProof, error) {
	trial, err := s.Get(id)
	if err != nil {
		return domain.ArchiveProof{}, err
	}
	if trial.Status != domain.Archived {
		return domain.ArchiveProof{}, errors.New("仅 ARCHIVED 批次可执行完整性核验")
	}
	return trial.VerifyArchive(time.Now().UTC()), nil
}
