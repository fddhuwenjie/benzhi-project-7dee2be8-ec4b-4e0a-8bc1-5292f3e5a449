package application

import (
	"seed-vigor-gate/internal/domain"
	"seed-vigor-gate/internal/store"
	"testing"
	"time"
)

func TestFlow(t *testing.T) {
	st, _ := store.New(t.TempDir())
	s := New(st)
	tr, e := s.Create("LOT1", "玉米", "owner", []string{"A"}, "tech", RoleTechnician, "r1")
	if e != nil {
		t.Fatal(e)
	}
	tr, e = s.LockProtocol(tr.ID, domain.Protocol{SampleSize: 2, AgeTempC: 40, AgeHumidity: 75, Rounds: 1, Threshold: 50, JudgementDate: "2026-01-01"}, tr.Revision, "lead", RoleLead, "r2")
	if e != nil {
		t.Fatal(e)
	}
	tr, e = s.RecordExposure(tr.ID, domain.Exposure{GroupCode: "A", StartedAt: tr.CreatedAt.Add(-time.Hour), EndedAt: tr.CreatedAt, Device: "TEST-01", TemperatureC: 40, Humidity: 75, Operator: "tech", Evidence: "测试读数"}, tr.Revision, "tech", RoleTechnician, "r3")
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.SubmitRound(tr.ID, domain.GerminationRound{GroupCode: "A", RoundNo: 1, NormalCount: 2, Operator: "tech"}, tr.Revision, "tech", RoleTechnician, "r4")
	if e != nil {
		t.Fatal(e)
	}
}
