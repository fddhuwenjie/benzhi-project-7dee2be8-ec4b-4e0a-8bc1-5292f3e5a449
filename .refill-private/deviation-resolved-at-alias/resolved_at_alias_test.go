package deviationresolvedatalias

import (
	"testing"
	"time"

	"seed-vigor-gate/internal/domain"
	"seed-vigor-gate/internal/store"
)

func TestDeviationResolvedAtReadMustNotAliasStore(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	original := time.Date(2026, time.January, 12, 9, 30, 0, 0, time.UTC)
	trial := &domain.VigorTrial{
		ID:       "TR-ALIAS-RESOLVED",
		Status:   domain.Counting,
		Revision: 0,
		Deviations: []domain.DeviationCase{{
			ID:          "DEV-RESOLVED",
			TrialID:     "TR-ALIAS-RESOLVED",
			Decision:    "ACCEPTED",
			ResolvedAt:  &original,
			Remediations: []domain.RemediationVersion{{
				Evidence: "原始整改证据",
			}},
			Decisions: []domain.DeviationDecision{{
				Decision: "ACCEPTED",
			}},
		}},
	}
	if err := st.Save(trial, "seed-resolved-alias"); err != nil {
		t.Fatal(err)
	}
	read, err := st.Get(trial.ID)
	if err != nil {
		t.Fatal(err)
	}
	if read.Deviations[0].ResolvedAt == nil {
		t.Fatal("测试数据缺少 resolved_at")
	}
	expected := original
	mutated := original.Add(48 * time.Hour)
	*read.Deviations[0].ResolvedAt = mutated
	read.Deviations[0].Remediations[0].Evidence = "调用方临时证据"
	read.Deviations[0].Decisions[0].Decision = "REJECTED"
	again, err := st.Get(trial.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Deviations[0].ResolvedAt == nil || !again.Deviations[0].ResolvedAt.Equal(expected) ||
		again.Deviations[0].Remediations[0].Evidence != "原始整改证据" ||
		again.Deviations[0].Decisions[0].Decision != "ACCEPTED" {
		t.Fatalf("调用方修改读取副本后污染了 Store: got resolved=%v remediation=%q decision=%q", again.Deviations[0].ResolvedAt, again.Deviations[0].Remediations[0].Evidence, again.Deviations[0].Decisions[0].Decision)
	}
}
