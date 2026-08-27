package auditdataaliasintegrity

import (
	"testing"

	"seed-vigor-gate/internal/application"
	"seed-vigor-gate/internal/store"
)

func TestAuditTimelineMutationMustNotCorruptStore(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := application.New(st)
	trial, err := app.Create("LOT-AUDIT", "水稻", "负责人", []string{"A"}, "技术员", application.RoleTechnician, "req-audit-alias")
	if err != nil {
		t.Fatal(err)
	}
	timeline, err := app.Timeline(trial.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(timeline) != 1 || timeline[0].Data == nil {
		t.Fatalf("unexpected initial timeline: %#v", timeline)
	}
	timeline[0].Data["seed_lot_code"] = "CALLER-ONLY-VALUE"
	stored, err := app.Timeline(trial.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := stored[0].Data["seed_lot_code"]; got != "LOT-AUDIT" {
		t.Fatalf("caller mutation leaked into stored audit event: got %v", got)
	}
	if err := st.Validate(); err != nil {
		t.Fatalf("caller mutation broke the persisted audit chain: %v", err)
	}
}
