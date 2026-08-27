package releasereportalias

import (
	"seed-vigor-gate/internal/application"
	"seed-vigor-gate/internal/domain"
	"seed-vigor-gate/internal/store"
	"testing"
	"time"
)

func TestReleaseReportReadMustNotAliasStore(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	trial := &domain.VigorTrial{
		ID: "TR-REPORT-ALIAS", SeedLotCode: "LOT-REPORT", CropName: "玉米", Owner: "负责人甲",
		Status: domain.Released, Revision: 1, CreatedAt: time.Unix(1700000000, 0).UTC(),
		ReleaseDecision: &domain.ReleaseDecision{Signer: "负责人乙", Report: domain.GateReport{
			Blocks: []domain.GateBlock{{Code: "REVIEW", Message: "复核记录", People: []string{"复核员甲"}}},
		}},
	}
	if err := st.Save(trial, "seed-report-alias"); err != nil {
		t.Fatal(err)
	}
	service := application.New(st)
	first, err := service.Get(trial.ID)
	if err != nil {
		t.Fatal(err)
	}
	first.ReleaseDecision.Report.Blocks[0].People[0] = "临时展示人员"
	second, err := service.Get(trial.ID)
	if err != nil {
		t.Fatal(err)
	}
	if second.ReleaseDecision.Report.Blocks[0].People[0] == "临时展示人员" {
		t.Fatalf("release report mutation leaked into store")
	}
}
