package querycachealias_test

import (
	"testing"

	"seed-vigor-gate/internal/application"
	"seed-vigor-gate/internal/domain"
	"seed-vigor-gate/internal/store"
)

func TestQueryCacheMustIsolateCallerMutation(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := application.New(st)
	created, err := service.Create(
		"LOT-CACHE-01",
		"玉米",
		"技术员甲",
		[]string{"A"},
		"技术员甲",
		application.RoleTechnician,
		"query-cache-create",
	)
	if err != nil {
		t.Fatal(err)
	}

	filter := application.TrialFilter{Status: domain.Draft, Limit: 20}
	first, err := service.Query(filter)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Trials) != 1 {
		t.Fatalf("首次查询返回 %d 条，期望 1 条", len(first.Trials))
	}

	// 模拟 HTTP 以外的调用方在渲染前给返回模型附加临时展示状态。
	first.Trials[0].SeedLotCode = "CALLER-MUTATED"
	first.Summary.ByStatus[domain.Draft] = 99

	second, err := service.Query(filter)
	if err != nil {
		t.Fatal(err)
	}
	if second.Trials[0].SeedLotCode != created.SeedLotCode {
		t.Fatalf("缓存向后续调用泄漏前一调用方修改: got %q want %q", second.Trials[0].SeedLotCode, created.SeedLotCode)
	}
	if second.Summary.ByStatus[domain.Draft] != 1 {
		t.Fatalf("缓存汇总被前一调用方污染: got %d want 1", second.Summary.ByStatus[domain.Draft])
	}
}
