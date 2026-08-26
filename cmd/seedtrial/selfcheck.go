package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"seed-vigor-gate/internal/domain"
)

func performSelfCheck(ctx context.Context, base string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	var t domain.VigorTrial
	post := func(method, path string, body any) error {
		b, _ := json.Marshal(body)
		req, e := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(b))
		if e != nil {
			return e
		}
		req.Header.Set("Content-Type", "application/json")
		resp, e := client.Do(req)
		if e != nil {
			return e
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 300 {
			return fmt.Errorf("%s %s: %s", method, path, raw)
		}
		return json.Unmarshal(raw, &t)
	}
	id := func(n int) string { return fmt.Sprintf("self-%d-%d", time.Now().UnixNano(), n) }
	if e := post("POST", "/api/trials", map[string]any{"seed_lot_code": "SELF-CHECK", "crop_name": "水稻", "owner": "技术员甲", "groups": []string{"A组"}, "actor": "技术员甲", "role": "TECHNICIAN", "request_id": id(1)}); e != nil {
		return e
	}
	if t.Status != domain.Draft {
		return errors.New("创建状态断言失败")
	}
	path := "/api/trials/" + t.ID
	protocol := map[string]any{"sample_size": 10, "aging_temp_c": 41, "aging_humidity": 75, "judgement_date": time.Now().UTC().Format("2006-01-02"), "rounds": 1, "threshold": 80, "actor": "负责人乙", "role": "LEAD", "request_id": id(2), "expected_revision": t.Revision}
	preflightBody, _ := json.Marshal(protocol)
	preflightRequest, e := http.NewRequestWithContext(ctx, "POST", base+path+"/protocol/preflight", bytes.NewReader(preflightBody))
	if e != nil {
		return e
	}
	preflightRequest.Header.Set("Content-Type", "application/json")
	preflightResponse, e := client.Do(preflightRequest)
	if e != nil {
		return e
	}
	var preflight domain.ProtocolPreflight
	e = json.NewDecoder(preflightResponse.Body).Decode(&preflight)
	preflightResponse.Body.Close()
	if e != nil || preflightResponse.StatusCode >= 300 || preflight.Summary == "" {
		return errors.New("方案预检失败")
	}
	protocol["preflight_summary"] = preflight.Summary
	if e := post("PUT", path+"/protocol", protocol); e != nil {
		return e
	}
	now := time.Now().UTC()
	if e := post("POST", path+"/exposures", map[string]any{"group_code": "A组", "started_at": now.Add(-24 * time.Hour), "ended_at": now, "device": "SELF-01", "temperature_c": 41, "humidity": 75, "operator": "技术员甲", "evidence": "自检读数", "actor": "技术员甲", "role": "TECHNICIAN", "request_id": id(3), "expected_revision": t.Revision}); e != nil {
		return e
	}
	if e := post("POST", path+"/rounds", map[string]any{"group_code": "A组", "round_no": 1, "normal_count": 9, "abnormal_count": 1, "ungerminated_count": 0, "operator": "技术员甲", "evidence_note": "自检计数", "actor": "技术员甲", "role": "TECHNICIAN", "request_id": id(4), "expected_revision": t.Revision}); e != nil {
		return e
	}
	if e := post("POST", path+"/review", map[string]any{"reviewer": "复核员丙", "actor": "复核员丙", "role": "REVIEWER", "request_id": id(5), "expected_revision": t.Revision}); e != nil {
		return e
	}
	if t.Status != domain.Reviewed {
		return fmt.Errorf("复核状态断言失败: %s", t.Status)
	}
	if e := post("POST", path+"/release", map[string]any{"actor": "负责人乙", "role": "LEAD", "request_id": id(6), "expected_revision": t.Revision}); e != nil {
		return e
	}
	if e := post("POST", path+"/archive", map[string]any{"actor": "负责人乙", "role": "LEAD", "request_id": id(7), "expected_revision": t.Revision}); e != nil {
		return e
	}
	if t.Status != domain.Archived || t.ArchiveDigest == "" {
		return errors.New("归档状态断言失败")
	}
	fmt.Printf("自检通过：%s 已完成创建、复核、放行和归档\n", t.ID)
	return nil
}
