package auditwriterrotation

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"seed-vigor-gate/internal/application"
	"seed-vigor-gate/internal/domain"
	"seed-vigor-gate/internal/store"
)

func TestRotatedAuditWriterMustFollowCurrentFile(t *testing.T) {
	dir := t.TempDir()
	st, err := store.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	service := application.New(st)
	trial, err := service.Create("ROTATE-LOT", "玉米", "技术员甲", []string{"A"}, "技术员甲", application.RoleTechnician, "rotate-create")
	if err != nil {
		t.Fatal(err)
	}

	auditPath := filepath.Join(dir, "audit.jsonl")
	beforeRotation, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(auditPath, auditPath+".1"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(auditPath, beforeRotation, 0644); err != nil {
		t.Fatal(err)
	}

	protocol := domain.Protocol{
		SampleSize: 20, AgeTempC: 41, AgeHumidity: 75,
		JudgementDate: time.Now().UTC().Format("2006-01-02"), Rounds: 1, Threshold: 80,
	}
	preflight, err := service.PreflightProtocol(trial.ID, protocol, trial.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.LockProtocolConfirmed(trial.ID, protocol, trial.Revision, preflight.Summary, "负责人乙", application.RoleLead, "rotate-lock"); err != nil {
		t.Fatalf("轮转后的命令应成功提交: %v", err)
	}

	events, err := store.ReadAudit(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("当前审计文件应包含两次成功命令，实际只有 %d 条", len(events))
	}
	if err = st.Validate(); err != nil {
		t.Fatalf("轮转后的当前审计文件应与已提交快照一致: %v", err)
	}
}
