package idempotencyscope_test

import (
	"testing"

	"seed-vigor-gate/internal/application"
	"seed-vigor-gate/internal/store"
)

func TestRequestIDMustNotReplayDifferentCommand(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := application.New(st)
	created, err := service.Create("LOT-A", "玉米", "技术员甲", []string{"A"}, "技术员甲", application.RoleTechnician, "shared-request")
	if err != nil {
		t.Fatal(err)
	}

	// Restarting clears the in-memory result cache and exercises persisted replay lookup.
	restarted := application.New(st)
	replayed, err := restarted.Release("missing-trial", -1, "", "", "shared-request")
	if err == nil && replayed != nil && replayed.ID == created.ID {
		t.Fatalf("不同资源和操作复用 request_id 时返回了无关批次 %s", replayed.ID)
	}
}
