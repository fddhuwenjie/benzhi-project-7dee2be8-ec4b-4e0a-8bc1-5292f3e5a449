package trailingjsonbody_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"seed-vigor-gate/internal/application"
	"seed-vigor-gate/internal/httpapi"
	"seed-vigor-gate/internal/store"
)

func TestTrailingJSONObjectIsRejectedWithoutMutation(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := application.New(st)
	handler := httpapi.New(service)
	body := []byte(`{"seed_lot_code":"LOT-JSON","crop_name":"玉米","owner":"技术员甲","groups":["A"],"actor":"技术员甲","role":"TECHNICIAN","request_id":"create"} {"second":"object"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/trials", bytes.NewReader(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code == http.StatusOK || len(service.List()) != 0 {
		t.Fatalf("尾随 JSON 对象被忽略并产生状态写入，HTTP %d", response.Code)
	}
}
