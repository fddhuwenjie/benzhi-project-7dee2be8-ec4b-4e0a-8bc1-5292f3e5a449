package canceledcreatecontext

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"seed-vigor-gate/internal/application"
	"seed-vigor-gate/internal/httpapi"
	"seed-vigor-gate/internal/store"
)

func TestCanceledCreateDoesNotPersist(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.New(application.New(st))
	payload, err := json.Marshal(map[string]any{
		"seed_lot_code": "CANCEL-LOT",
		"crop_name":     "玉米",
		"owner":         "技术员甲",
		"groups":        []string{"A"},
		"actor":         "技术员甲",
		"role":          "TECHNICIAN",
		"request_id":    "cancel-create-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest("POST", "/api/trials", bytes.NewReader(payload)).WithContext(ctx)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code == 200 {
		t.Errorf("TestCanceledCreateDoesNotPersist: canceled request returned success: %s", resp.Body.String())
	}
	if trials := st.List(); len(trials) != 0 {
		t.Fatalf("TestCanceledCreateDoesNotPersist: canceled request persisted %d trial(s)", len(trials))
	}
}
