package application

import (
	"errors"
	"testing"
	"time"

	"seed-vigor-gate/internal/domain"
	"seed-vigor-gate/internal/store"
)

func newFeatureService(t *testing.T) *Service {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return New(st)
}

func createAndLock(t *testing.T, service *Service, groups []string, rounds int, threshold float64) *domain.VigorTrial {
	t.Helper()
	trial, err := service.Create("LOT-Alpha", "玉米", "技术员甲", groups, "技术员甲", RoleTechnician, "create")
	if err != nil {
		t.Fatal(err)
	}
	protocol := domain.Protocol{SampleSize: 100, AgeTempC: 41, AgeHumidity: 75, JudgementDate: time.Now().UTC().Format("2006-01-02"), Rounds: rounds, Threshold: threshold}
	preflight, err := service.PreflightProtocol(trial.ID, protocol, trial.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if preflight.ExpectedCountRecords != len(groups)*rounds {
		t.Fatalf("预计计数记录错误: %d", preflight.ExpectedCountRecords)
	}
	trial, err = service.LockProtocolConfirmed(trial.ID, protocol, trial.Revision, preflight.Summary, "负责人乙", RoleLead, "lock")
	if err != nil {
		t.Fatal(err)
	}
	return trial
}

func recordExposure(t *testing.T, service *Service, trial *domain.VigorTrial, group, request string, temperature, humidity float64) *domain.VigorTrial {
	t.Helper()
	now := time.Now().UTC().Add(-time.Second)
	result, err := service.RecordExposure(trial.ID, domain.Exposure{GroupCode: group, StartedAt: now.Add(-24 * time.Hour), EndedAt: now, Device: "CH-01", TemperatureC: temperature, Humidity: humidity, Operator: "技术员甲", Evidence: "设备导出记录"}, trial.Revision, "技术员甲", RoleTechnician, request)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestQueryPreflightAndReadOnlySummary(t *testing.T) {
	service := newFeatureService(t)
	first, err := service.Create("Lot-One", "Corn", "Alice", []string{"A"}, "tech", RoleTechnician, "q1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Create("Lot-Two", "Rice", "Bob", []string{"B"}, "tech", RoleTechnician, "q2")
	if err != nil {
		t.Fatal(err)
	}
	protocol := domain.Protocol{SampleSize: 20, AgeTempC: 40, AgeHumidity: 70, JudgementDate: time.Now().UTC().Format("2006-01-02"), Rounds: 3, Threshold: 80}
	preflight, err := service.PreflightProtocol(first.ID, protocol, first.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if preflight.ExpectedCountRecords != 3 || preflight.TemperatureRange.Minimum != 39 || preflight.HumidityRange.Maximum != 75 {
		t.Fatalf("预检摘要错误: %+v", preflight)
	}
	after, _ := service.Get(first.ID)
	if after.Revision != first.Revision {
		t.Fatal("预检不应增加 revision")
	}
	page, err := service.Query(TrialFilter{SeedLotCode: "lot", CropName: "CORN", Owner: "ali", Status: domain.Draft, Limit: 10})
	if err != nil || page.Total != 1 || page.Trials[0].ID != first.ID || page.Summary.MissingRounds != 0 {
		t.Fatalf("组合查询错误: %+v %v", page, err)
	}
	if _, err = service.Query(TrialFilter{Offset: -1, Limit: 10}); err == nil {
		t.Fatal("负 offset 应被拒绝")
	}
	if _, err = service.Query(TrialFilter{Limit: MaxPageLimit + 1}); err == nil {
		t.Fatal("超限 limit 应被拒绝")
	}
	if _, err = service.LockProtocolConfirmed(first.ID, protocol, first.Revision, "stale", "lead", RoleLead, "bad-lock"); err == nil {
		t.Fatal("内容不一致的摘要应被拒绝")
	}
}

func TestExposureItemsSequenceAndIdempotency(t *testing.T) {
	service := newFeatureService(t)
	trial := createAndLock(t, service, []string{"A", "B"}, 2, 70)
	trial = recordExposure(t, service, trial, "A", "exposure-a", 43.5, 65)
	if trial.Status != domain.ProtocolLocked || len(trial.Deviations) != 2 {
		t.Fatalf("分项偏差或登记进度错误: %s %d", trial.Status, len(trial.Deviations))
	}
	for _, deviation := range trial.Deviations {
		if deviation.GroupCode != "A" || deviation.Device != "CH-01" || !deviation.Automatic {
			t.Fatalf("偏差上下文不完整: %+v", deviation)
		}
	}
	replayed, err := service.RecordExposure(trial.ID, domain.Exposure{}, -1, "技术员甲", RoleTechnician, "exposure-a")
	if err != nil || replayed.Revision != trial.Revision || len(replayed.Deviations) != 2 {
		t.Fatal("request_id 重放不应重复创建偏差")
	}
	trial = recordExposure(t, service, trial, "B", "exposure-b", 41, 75)
	if trial.Status != domain.ExposureRecorded {
		t.Fatalf("补齐条件后状态错误: %s", trial.Status)
	}
	_, err = service.SubmitRound(trial.ID, domain.GerminationRound{GroupCode: "A", RoundNo: 2, NormalCount: 80, AbnormalCount: 10, UngerminatedCount: 10, Operator: "技术员甲"}, trial.Revision, "技术员甲", RoleTechnician, "round-a2-first")
	if err == nil {
		t.Fatal("跳轮应被拒绝")
	}
	after, _ := service.Get(trial.ID)
	if after.Revision != trial.Revision || len(after.Rounds) != 0 {
		t.Fatal("跳轮失败不应写入")
	}
	trial, err = service.SubmitRound(trial.ID, domain.GerminationRound{GroupCode: "A", RoundNo: 1, NormalCount: 70, AbnormalCount: 10, UngerminatedCount: 20, Operator: "技术员甲"}, trial.Revision, "技术员甲", RoleTechnician, "round-a1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.SubmitRound(trial.ID, domain.GerminationRound{GroupCode: "A", RoundNo: 2, NormalCount: 69, AbnormalCount: 10, UngerminatedCount: 21, Operator: "技术员甲"}, trial.Revision, "技术员甲", RoleTechnician, "round-a2-bad")
	if err == nil {
		t.Fatal("累计单调性错误应被拒绝")
	}
}

func TestCorrectionRemediationAndSeparation(t *testing.T) {
	service := newFeatureService(t)
	trial := createAndLock(t, service, []string{"A"}, 2, 50)
	trial = recordExposure(t, service, trial, "A", "c-exposure", 41, 75)
	var err error
	trial, err = service.SubmitRound(trial.ID, domain.GerminationRound{GroupCode: "A", RoundNo: 1, NormalCount: 70, AbnormalCount: 10, UngerminatedCount: 20, Operator: "技术员甲"}, trial.Revision, "技术员甲", RoleTechnician, "c-round-1")
	if err != nil {
		t.Fatal(err)
	}
	trial, err = service.SubmitRound(trial.ID, domain.GerminationRound{GroupCode: "A", RoundNo: 2, NormalCount: 80, AbnormalCount: 10, UngerminatedCount: 10, Operator: "技术员甲"}, trial.Revision, "技术员甲", RoleTechnician, "c-round-2")
	if err != nil {
		t.Fatal(err)
	}
	beforeRevision := trial.Revision
	_, err = service.CorrectRound(trial.ID, "A", 1, 85, 5, 10, "录入错误", "原始记录", trial.Revision, "技术员甲", RoleTechnician, "bad-correction")
	if err == nil {
		t.Fatal("破坏下一轮单调性的更正应被拒绝")
	}
	after, _ := service.Get(trial.ID)
	if after.Revision != beforeRevision || len(after.Corrections) != 0 {
		t.Fatal("失败更正不应留下历史")
	}
	trial, err = service.CorrectRound(trial.ID, "A", 1, 75, 10, 15, "录入错误", "原始记录", trial.Revision, "技术员甲", RoleTechnician, "good-correction")
	if err != nil || len(trial.Corrections) != 1 || trial.Corrections[0].Before.NormalCount != 70 {
		t.Fatalf("更正版本未保留: %v %+v", err, trial.Corrections)
	}
	if _, err = service.ReviewCounts(trial.ID, "技术员甲", trial.Revision, "技术员甲", RoleReviewer, "self-review"); err == nil {
		t.Fatal("计数人不得复核自己提交的记录")
	}
	trial, err = service.ReviewCounts(trial.ID, "复核员丙", trial.Revision, "复核员丙", RoleReviewer, "independent-review")
	if err != nil || trial.Status != domain.Reviewed {
		t.Fatalf("独立复核失败: %v %s", err, trial.Status)
	}
	report, _ := service.GatePreview(trial.ID, "技术员甲")
	if report.SeparationSatisfied {
		t.Fatal("owner 签发应被职责分离阻断")
	}
	trial, err = service.Release(trial.ID, trial.Revision, "负责人乙", RoleLead, "release")
	if err != nil || trial.ReleaseDecision == nil || trial.ReleaseDecision.Digest == "" {
		t.Fatalf("决定快照缺失: %v %+v", err, trial.ReleaseDecision)
	}
}

func TestRejectedDeviationRemediationAndArchiveProofReadOnly(t *testing.T) {
	service := newFeatureService(t)
	trial := createAndLock(t, service, []string{"A"}, 1, 50)
	trial = recordExposure(t, service, trial, "A", "r-exposure", 43, 75)
	trial, _ = service.SubmitRound(trial.ID, domain.GerminationRound{GroupCode: "A", RoundNo: 1, NormalCount: 90, AbnormalCount: 5, UngerminatedCount: 5, Operator: "技术员甲"}, trial.Revision, "技术员甲", RoleTechnician, "r-round")
	trial, _ = service.ReviewCounts(trial.ID, "复核员丙", trial.Revision, "复核员丙", RoleReviewer, "r-review-count")
	deviationID := trial.Deviations[0].ID
	trial, _ = service.ReviewDeviation(trial.ID, deviationID, "REJECTED", "设备漂移", "重新核对", "复核员丙", trial.Revision, "复核员丙", RoleReviewer, "r-reject")
	if trial.Status != domain.Counting || trial.Deviations[0].Stage != domain.DeviationRemediation {
		t.Fatal("退回偏差必须保持 COUNTING 并进入待整改")
	}
	trial, err := service.RemediateDeviation(trial.ID, deviationID, "探头漂移", "复校确认可接受", "校准单-01", trial.Revision, "技术员甲", RoleTechnician, "r-remediate")
	if err != nil || len(trial.Deviations[0].Remediations) != 1 {
		t.Fatal("整改版本未保存")
	}
	replay, err := service.RemediateDeviation(trial.ID, deviationID, "x", "x", "x", -1, "技术员甲", RoleTechnician, "r-remediate")
	if err != nil || replay.Revision != trial.Revision || len(replay.Deviations[0].Remediations) != 1 {
		t.Fatal("整改重放不应新增版本")
	}
	trial, _ = service.ReviewDeviation(trial.ID, deviationID, "ACCEPTED", "探头漂移", "证据充分", "复核员丁", trial.Revision, "复核员丁", RoleReviewer, "r-accept")
	if trial.Status != domain.Reviewed || len(trial.Deviations[0].Decisions) != 2 {
		t.Fatal("复裁接受后应进入 REVIEWED 并保留两轮裁决")
	}
	if _, err = service.Release(trial.ID, trial.Revision, "复核员丁", RoleLead, "bad-release"); err == nil {
		t.Fatal("偏差复核员不得签发")
	}
	trial, _ = service.Release(trial.ID, trial.Revision, "负责人乙", RoleLead, "r-release")
	trial, err = service.Archive(trial.ID, trial.Revision, "负责人乙", RoleLead, "r-archive")
	if err != nil {
		t.Fatal(err)
	}
	if got := trial.Audit[len(trial.Audit)-1].Data["archive_digest"]; got != trial.ArchiveDigest {
		t.Fatalf("归档审计事件未保存最终摘要: %v != %s", got, trial.ArchiveDigest)
	}
	revision, eventCount := trial.Revision, len(trial.Audit)
	proof, err := service.ArchiveProof(trial.ID)
	if err != nil || !proof.Passed {
		t.Fatalf("正常归档证明应通过: %v %+v", err, proof)
	}
	afterProof, _ := service.Get(trial.ID)
	if afterProof.Revision != revision || len(afterProof.Audit) != eventCount {
		t.Fatal("归档核验必须只读")
	}
	if !errors.Is(afterProof.VerifyIntegrity(), nil) {
		t.Fatal("归档审计链应完整")
	}
}

func TestAcceptedThresholdDeviationAllowsGate(t *testing.T) {
	service := newFeatureService(t)
	trial := createAndLock(t, service, []string{"A"}, 1, 80)
	trial = recordExposure(t, service, trial, "A", "threshold-exposure", 41, 75)
	trial, _ = service.SubmitRound(trial.ID, domain.GerminationRound{GroupCode: "A", RoundNo: 1, NormalCount: 70, AbnormalCount: 10, UngerminatedCount: 20, Operator: "技术员甲"}, trial.Revision, "技术员甲", RoleTechnician, "threshold-round")
	trial, _ = service.ReviewCounts(trial.ID, "复核员丙", trial.Revision, "复核员丙", RoleReviewer, "threshold-review")
	if len(trial.Deviations) != 1 || trial.Status != domain.Counting {
		t.Fatalf("低阈值自动偏差未生成: %+v", trial.Deviations)
	}
	trial, err := service.ReviewDeviation(trial.ID, trial.Deviations[0].ID, "ACCEPTED", "样品固有差异", "批准例外", "复核员丙", trial.Revision, "复核员丙", RoleReviewer, "threshold-accept")
	if err != nil || trial.Status != domain.Reviewed {
		t.Fatalf("阈值偏差接受后未完成复核: %v %s", err, trial.Status)
	}
	report, err := service.GatePreview(trial.ID, "负责人乙")
	if err != nil || !report.Passed || !report.ThresholdSatisfied {
		t.Fatalf("已批准阈值例外应通过门禁: %v %+v", err, report)
	}
}
