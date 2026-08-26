package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"seed-vigor-gate/internal/application"
	"seed-vigor-gate/internal/domain"
	"seed-vigor-gate/internal/store"
	"testing"
)

func TestHome(t *testing.T) {
	st, _ := store.New(t.TempDir())
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	New(application.New(st)).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w.Body.Len() < 100 {
		t.Fatal("empty html")
	}
}

func TestTrialQueryValidationAndProtocolPreflight(t *testing.T) {
	st, _ := store.New(t.TempDir())
	service := application.New(st)
	handler := New(service)
	trial, err := service.Create("HTTP-LOT", "玉米", "技术员甲", []string{"A", "B"}, "技术员甲", application.RoleTechnician, "http-create")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/trials?offset=-1", "/api/trials?limit=101", "/api/trials?status=UNKNOWN"} {
		request := httptest.NewRequest("GET", path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != 400 {
			t.Fatalf("%s 应返回 400，实际 %d", path, response.Code)
		}
	}
	queryRequest := httptest.NewRequest("GET", "/api/trials?seed_lot_code=http&crop_name=%E7%8E%89%E7%B1%B3&owner=%E6%8A%80%E6%9C%AF&limit=10", nil)
	queryResponse := httptest.NewRecorder()
	handler.ServeHTTP(queryResponse, queryRequest)
	var page application.TrialPage
	if err = json.Unmarshal(queryResponse.Body.Bytes(), &page); err != nil || page.Total != 1 || page.Trials[0].ID != trial.ID {
		t.Fatalf("HTTP 组合查询错误: %v %s", err, queryResponse.Body.String())
	}
	payload := map[string]any{"sample_size": 100, "aging_temp_c": 41, "aging_humidity": 75, "judgement_date": "2026-08-26", "rounds": 3, "threshold": 80, "expected_revision": trial.Revision, "actor": "负责人乙", "role": "LEAD", "request_id": "preflight"}
	body, _ := json.Marshal(payload)
	preflightRequest := httptest.NewRequest("POST", "/api/trials/"+trial.ID+"/protocol/preflight", bytes.NewReader(body))
	preflightResponse := httptest.NewRecorder()
	handler.ServeHTTP(preflightResponse, preflightRequest)
	var preflight domain.ProtocolPreflight
	if err = json.Unmarshal(preflightResponse.Body.Bytes(), &preflight); err != nil || preflight.ExpectedCountRecords != 6 || preflight.Summary == "" {
		t.Fatalf("HTTP 预检错误: %v %s", err, preflightResponse.Body.String())
	}
	after, _ := service.Get(trial.ID)
	if after.Revision != trial.Revision {
		t.Fatal("HTTP 预检不得修改 revision")
	}
}
