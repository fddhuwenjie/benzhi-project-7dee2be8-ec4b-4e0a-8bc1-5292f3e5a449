package archivechecklistalias

import (
	"testing"
	"time"

	"seed-vigor-gate/internal/application"
	"seed-vigor-gate/internal/domain"
	"seed-vigor-gate/internal/store"
)

func TestArchiveChecklistReadMustNotAliasStore(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	releasedAt := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	trial := &domain.VigorTrial{
		ID:            "TR-ARCHIVE-ALIAS",
		SeedLotCode:   "LOT-ARCHIVE",
		CropName:      "水稻",
		Owner:         "负责人甲",
		Status:        domain.Archived,
		Revision:      0,
		ArchiveDigest: "sealed",
		ReleasedAt:    &releasedAt,
		SealedChecklist: &domain.ArchiveChecklist{
			TrialID: "TR-ARCHIVE-ALIAS",
			Items:   []string{"原始清单项"},
		},
	}
	expectedReleasedAt := *trial.ReleasedAt
	trial.SealedChecklist.TrialID = trial.ID
	if err := st.Save(trial, "archive-seed"); err != nil {
		t.Fatal(err)
	}
	service := application.New(st)
	read, err := service.Get(trial.ID)
	if err != nil {
		t.Fatal(err)
	}
	read.SealedChecklist.Items[0] = "调用方临时展示值"
	*read.ReleasedAt = read.ReleasedAt.Add(24 * time.Hour)
	again, err := service.Get(trial.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := again.SealedChecklist.Items[0]; got != "原始清单项" {
		t.Fatalf("TestArchiveChecklistReadMustNotAliasStore: store state was polluted, got %q", got)
	}
	if !again.ReleasedAt.Equal(expectedReleasedAt) {
		t.Fatalf("TestArchiveChecklistReadMustNotAliasStore: released_at was polluted, got %s", again.ReleasedAt)
	}
}
