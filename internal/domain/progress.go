package domain

import (
	"sort"
	"strconv"
)

type GroupProgress struct {
	GroupCode        string            `json:"group_code"`
	ExposureRecorded bool              `json:"exposure_recorded"`
	SubmittedRounds  []int             `json:"submitted_rounds"`
	MissingRounds    []int             `json:"missing_rounds"`
	LatestVigorRate  float64           `json:"latest_vigor_rate"`
	BelowThreshold   bool              `json:"below_threshold"`
	NextRound        int               `json:"next_round,omitempty"`
	RemainingRounds  int               `json:"remaining_rounds"`
	PreviousRound    *GerminationRound `json:"previous_round,omitempty"`
}

type TrialProgress struct {
	TotalGroups        int             `json:"total_groups"`
	ExposureGroups     int             `json:"exposure_groups"`
	ExpectedRounds     int             `json:"expected_rounds"`
	SubmittedRounds    int             `json:"submitted_rounds"`
	ResolvedDeviations int             `json:"resolved_deviations"`
	OpenDeviations     int             `json:"open_deviations"`
	Percent            int             `json:"percent"`
	Groups             []GroupProgress `json:"groups"`
}

func (t *VigorTrial) Progress() TrialProgress {
	progress := TrialProgress{TotalGroups: len(t.Groups), ExpectedRounds: len(t.Groups) * t.Protocol.Rounds, SubmittedRounds: len(t.Rounds)}
	for _, deviation := range t.Deviations {
		if deviation.Decision != "ACCEPTED" {
			progress.OpenDeviations++
		} else {
			progress.ResolvedDeviations++
		}
	}
	for _, code := range t.Groups {
		group := GroupProgress{GroupCode: code}
		for _, exposure := range t.Exposures {
			if exposure.GroupCode == code {
				group.ExposureRecorded = true
				progress.ExposureGroups++
				break
			}
		}
		seen := map[int]bool{}
		for _, round := range t.Rounds {
			if round.GroupCode != code {
				continue
			}
			seen[round.RoundNo] = true
			group.SubmittedRounds = append(group.SubmittedRounds, round.RoundNo)
			if round.RoundNo >= len(group.SubmittedRounds) {
				group.LatestVigorRate = round.VigorRate
			}
			if round.VigorRate < t.Protocol.Threshold {
				group.BelowThreshold = true
			}
		}
		for number := 1; number <= t.Protocol.Rounds; number++ {
			if !seen[number] {
				group.MissingRounds = append(group.MissingRounds, number)
			}
		}
		sort.Ints(group.SubmittedRounds)
		if len(group.MissingRounds) > 0 {
			group.NextRound = group.MissingRounds[0]
			group.RemainingRounds = len(group.MissingRounds)
			if previous, ok := t.Round(code, group.NextRound-1); ok {
				copy := previous
				group.PreviousRound = &copy
			}
		}
		progress.Groups = append(progress.Groups, group)
	}
	weights := map[Status]int{Draft: 5, ProtocolLocked: 20, ExposureRecorded: 40, Counting: 60, Reviewed: 80, Released: 90, Archived: 100}
	progress.Percent = weights[t.Status]
	return progress
}

func (t *VigorTrial) MissingRoundKeys() []string {
	var keys []string
	for _, group := range t.Progress().Groups {
		for _, number := range group.MissingRounds {
			keys = append(keys, RoundKey(group.GroupCode, number))
		}
	}
	return keys
}

func RoundKey(group string, number int) string { return group + "#" + strconv.Itoa(number) }
