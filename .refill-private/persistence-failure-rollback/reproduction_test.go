package persistence_failure_rollback_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"seed-vigor-gate/internal/application"
	"seed-vigor-gate/internal/httpapi"
	"seed-vigor-gate/internal/store"
)

func TestFailedPersistenceMustRollbackMemoryAndIdempotency(t *testing.T) {
	dataDir := t.TempDir()
	st, err := store.New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.New(application.New(st))
	snapshotPath := filepath.Join(dataDir, "snapshot.json")
	if err = os.Mkdir(snapshotPath, 0o755); err != nil {
		t.Fatal(err)
	}

	body, err := json.Marshal(map[string]any{
		"seed_lot_code": "ROLLBACK-LOT",
		"crop_name":     "水稻",
		"owner":         "技术员甲",
		"groups":        []string{"A组"},
		"actor":         "技术员甲",
		"role":          application.RoleTechnician,
		"request_id":    "rollback-create",
	})
	if err != nil {
		t.Fatal(err)
	}
	sendCreate := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/trials", bytes.NewReader(body))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}

	first := sendCreate()
	if first.Code == http.StatusOK {
		t.Fatalf("首次快照写入受阻时不应成功: %s", first.Body.String())
	}
	query := httptest.NewRequest(http.MethodGet, "/api/trials?limit=20", nil)
	queryResponse := httptest.NewRecorder()
	handler.ServeHTTP(queryResponse, query)
	var page application.TrialPage
	if err = json.Unmarshal(queryResponse.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	_, requestLeaked := st.Seen("rollback-create")

	if err = os.Remove(snapshotPath); err != nil {
		t.Fatal(err)
	}
	retry := sendCreate()
	restarted, restartErr := store.New(dataDir)
	restartedCount := -1
	if restartErr == nil {
		restartedCount = len(restarted.List())
	}

	if page.Total != 0 || requestLeaked || retry.Code != http.StatusOK || restartErr != nil || restartedCount != 1 {
		t.Fatalf("失败事务污染了内存或幂等重放，导致重启后数据丢失: total=%d seen=%t retry=%d restart_err=%v restarted=%d",
			page.Total, requestLeaked, retry.Code, restartErr, restartedCount)
	}
}
