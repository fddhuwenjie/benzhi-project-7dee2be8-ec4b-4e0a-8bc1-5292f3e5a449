package httpapi

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"seed-vigor-gate/internal/application"
	"seed-vigor-gate/internal/domain"
)

type Server struct {
	app *application.Service
	mux *http.ServeMux
}

func New(app *application.Service) http.Handler {
	s := &Server{app: app, mux: http.NewServeMux()}
	s.routes()
	return withSecurityHeaders(s)
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }
func (s *Server) routes() {
	webFS, _ := fs.Sub(assets, "web")
	s.mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(webFS))))
	s.mux.HandleFunc("/", s.Home)
	s.mux.HandleFunc("/api/trials", s.Trials)
	s.mux.HandleFunc("/api/trials/", s.TrialActions)
}
func (s *Server) Home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != "GET" {
		method(w)
		return
	}
	b, _ := assets.ReadFile("web/index.html")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b)
}
func (s *Server) Trials(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		s.ListTrials(w, r)
	case "POST":
		s.CreateTrial(w, r)
	default:
		method(w)
	}
}

var validStatuses = map[domain.Status]bool{
	domain.Draft: true, domain.ProtocolLocked: true, domain.ExposureRecorded: true,
	domain.Counting: true, domain.Reviewed: true, domain.Released: true, domain.Archived: true,
}

func (s *Server) ListTrials(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	offset, err := queryInt(query.Get("offset"), 0, "offset")
	if err != nil {
		fail(w, err)
		return
	}
	limit, err := queryInt(query.Get("limit"), 20, "limit")
	if err != nil {
		fail(w, err)
		return
	}
	status := domain.Status(query.Get("status"))
	if status != "" && !validStatuses[status] {
		fail(w, errors.New("status 为未知状态"))
		return
	}
	page, err := s.app.Query(application.TrialFilter{SeedLotCode: query.Get("seed_lot_code"), CropName: query.Get("crop_name"), Owner: query.Get("owner"), Status: status, Offset: offset, Limit: limit})
	if err != nil {
		fail(w, err)
		return
	}
	write(w, 200, page)
}

func queryInt(raw string, fallback int, field string) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New(field + " 必须为整数")
	}
	return value, nil
}

type command struct {
	RequestID        string `json:"request_id"`
	ExpectedRevision int    `json:"expected_revision"`
	Actor            string `json:"actor"`
	Role             string `json:"role"`
}

type protocolRequest struct {
	command
	SampleSize       int     `json:"sample_size"`
	AgingTempC       float64 `json:"aging_temp_c"`
	AgingHumidity    float64 `json:"aging_humidity"`
	JudgementDate    string  `json:"judgement_date"`
	Rounds           int     `json:"rounds"`
	Threshold        float64 `json:"threshold"`
	PreflightSummary string  `json:"preflight_summary,omitempty"`
}

func (x protocolRequest) protocol() domain.Protocol {
	return domain.Protocol{SampleSize: x.SampleSize, AgeTempC: x.AgingTempC, AgeHumidity: x.AgingHumidity, JudgementDate: x.JudgementDate, Rounds: x.Rounds, Threshold: x.Threshold}
}

func decode(r *http.Request, v any) error {
	d := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	d.DisallowUnknownFields()
	return d.Decode(v)
}
func (s *Server) CreateTrial(w http.ResponseWriter, r *http.Request) {
	var x struct {
		command
		SeedLotCode string   `json:"seed_lot_code"`
		CropName    string   `json:"crop_name"`
		Owner       string   `json:"owner"`
		Groups      []string `json:"groups"`
	}
	if e := decode(r, &x); e != nil {
		fail(w, e)
		return
	}
	t, e := s.app.CreateContext(r.Context(), x.SeedLotCode, x.CropName, x.Owner, x.Groups, x.Actor, x.Role, x.RequestID)
	result(w, t, e)
}
func parts(path string) []string { return strings.Split(strings.Trim(path, "/"), "/") }
func (s *Server) TrialActions(w http.ResponseWriter, r *http.Request) {
	p := parts(r.URL.Path)
	if len(p) < 3 {
		http.NotFound(w, r)
		return
	}
	id := p[2]
	if len(p) == 3 && r.Method == "GET" {
		s.GetTrial(w, r, id)
		return
	}
	if len(p) < 4 {
		method(w)
		return
	}
	switch p[3] {
	case "protocol":
		if len(p) == 5 && p[4] == "preflight" {
			s.PreflightProtocol(w, r, id)
		} else {
			s.LockProtocol(w, r, id)
		}
	case "exposures":
		s.RecordExposure(w, r, id)
	case "rounds":
		if len(p) == 5 && p[4] == "correction" {
			s.CorrectRound(w, r, id)
		} else {
			s.SubmitRound(w, r, id)
		}
	case "review":
		s.ReviewCounts(w, r, id)
	case "deviations":
		if len(p) == 6 && p[5] == "review" {
			s.ReviewDeviation(w, r, id, p[4])
		} else if len(p) == 6 && p[5] == "remediation" {
			s.RemediateDeviation(w, r, id, p[4])
		} else {
			s.ReportDeviation(w, r, id)
		}
	case "release":
		s.ReleaseTrial(w, r, id)
	case "gate":
		s.GatePreview(w, r, id)
	case "archive":
		s.ArchiveTrial(w, r, id)
	case "audit":
		s.AuditTimeline(w, r, id)
	case "view":
		s.TrialView(w, r, id)
	case "checklist":
		s.ArchiveChecklist(w, r, id)
	case "integrity":
		s.ArchiveIntegrity(w, r, id)
	default:
		http.NotFound(w, r)
	}
}
func (s *Server) GetTrial(w http.ResponseWriter, r *http.Request, id string) {
	t, e := s.app.Get(id)
	result(w, t, e)
}
func (s *Server) LockProtocol(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "PUT" {
		method(w)
		return
	}
	var x protocolRequest
	if e := decode(r, &x); e != nil {
		fail(w, e)
		return
	}
	t, e := s.app.LockProtocolConfirmed(id, x.protocol(), x.ExpectedRevision, x.PreflightSummary, x.Actor, x.Role, x.RequestID)
	result(w, t, e)
}

func (s *Server) PreflightProtocol(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "POST" {
		method(w)
		return
	}
	var x protocolRequest
	if err := decode(r, &x); err != nil {
		fail(w, err)
		return
	}
	preflight, err := s.app.PreflightProtocol(id, x.protocol(), x.ExpectedRevision)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, 200, preflight)
}
func (s *Server) RecordExposure(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "POST" {
		method(w)
		return
	}
	var x struct {
		command
		domain.Exposure
	}
	if e := decode(r, &x); e != nil {
		fail(w, e)
		return
	}
	t, e := s.app.RecordExposure(id, x.Exposure, x.ExpectedRevision, x.Actor, x.Role, x.RequestID)
	if e != nil {
		fail(w, e)
		return
	}
	view, e := s.app.View(id)
	if e != nil {
		fail(w, e)
		return
	}
	type exposureResult struct {
		*domain.VigorTrial
		Progress               domain.TrialProgress   `json:"progress"`
		RecordedExposureGroups []string               `json:"recorded_exposure_groups"`
		PendingExposureGroups  []string               `json:"pending_exposure_groups"`
		AutomaticDeviations    []domain.DeviationCase `json:"automatic_deviations"`
	}
	automatic := make([]domain.DeviationCase, 0)
	for _, deviation := range t.Deviations {
		if deviation.Automatic && deviation.GroupCode == x.GroupCode {
			automatic = append(automatic, deviation)
		}
	}
	write(w, 200, exposureResult{VigorTrial: t, Progress: view.Progress, RecordedExposureGroups: view.RecordedExposureGroups, PendingExposureGroups: view.PendingExposureGroups, AutomaticDeviations: automatic})
}
func (s *Server) SubmitRound(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "POST" {
		method(w)
		return
	}
	var x struct {
		command
		GroupCode         string `json:"group_code"`
		RoundNo           int    `json:"round_no"`
		NormalCount       int    `json:"normal_count"`
		AbnormalCount     int    `json:"abnormal_count"`
		UngerminatedCount int    `json:"ungerminated_count"`
		Operator          string `json:"operator"`
		EvidenceNote      string `json:"evidence_note"`
	}
	if e := decode(r, &x); e != nil {
		fail(w, e)
		return
	}
	t, e := s.app.SubmitRound(id, domain.GerminationRound{GroupCode: x.GroupCode, RoundNo: x.RoundNo, NormalCount: x.NormalCount, AbnormalCount: x.AbnormalCount, UngerminatedCount: x.UngerminatedCount, Operator: x.Operator, EvidenceNote: x.EvidenceNote}, x.ExpectedRevision, x.Actor, x.Role, x.RequestID)
	result(w, t, e)
}

func (s *Server) CorrectRound(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "POST" {
		method(w)
		return
	}
	var x struct {
		command
		GroupCode         string `json:"group_code"`
		RoundNo           int    `json:"round_no"`
		NormalCount       int    `json:"normal_count"`
		AbnormalCount     int    `json:"abnormal_count"`
		UngerminatedCount int    `json:"ungerminated_count"`
		Reason            string `json:"reason"`
		Evidence          string `json:"evidence"`
	}
	if err := decode(r, &x); err != nil {
		fail(w, err)
		return
	}
	trial, err := s.app.CorrectRound(id, x.GroupCode, x.RoundNo, x.NormalCount, x.AbnormalCount, x.UngerminatedCount, x.Reason, x.Evidence, x.ExpectedRevision, x.Actor, x.Role, x.RequestID)
	result(w, trial, err)
}
func (s *Server) ReviewCounts(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "POST" {
		method(w)
		return
	}
	var x struct {
		command
		Reviewer string `json:"reviewer"`
	}
	if e := decode(r, &x); e != nil {
		fail(w, e)
		return
	}
	t, e := s.app.ReviewCounts(id, x.Reviewer, x.ExpectedRevision, x.Actor, x.Role, x.RequestID)
	result(w, t, e)
}
func (s *Server) ReportDeviation(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "POST" {
		method(w)
		return
	}
	var x struct {
		command
		Kind        string `json:"kind"`
		Severity    string `json:"severity"`
		RootCause   string `json:"root_cause"`
		Disposition string `json:"disposition"`
	}
	if e := decode(r, &x); e != nil {
		fail(w, e)
		return
	}
	t, e := s.app.ReportDeviation(id, domain.DeviationCase{Kind: x.Kind, Severity: x.Severity, RootCause: x.RootCause, Disposition: x.Disposition}, x.ExpectedRevision, x.Actor, x.Role, x.RequestID)
	result(w, t, e)
}
func (s *Server) ReviewDeviation(w http.ResponseWriter, r *http.Request, id, dev string) {
	if r.Method != "POST" {
		method(w)
		return
	}
	var x struct {
		command
		Decision    string `json:"decision"`
		RootCause   string `json:"root_cause"`
		Disposition string `json:"disposition"`
		Reviewer    string `json:"reviewer"`
	}
	if e := decode(r, &x); e != nil {
		fail(w, e)
		return
	}
	t, e := s.app.ReviewDeviation(id, dev, x.Decision, x.RootCause, x.Disposition, x.Reviewer, x.ExpectedRevision, x.Actor, x.Role, x.RequestID)
	result(w, t, e)
}

func (s *Server) RemediateDeviation(w http.ResponseWriter, r *http.Request, id, dev string) {
	if r.Method != "POST" {
		method(w)
		return
	}
	var x struct {
		command
		RootCause   string `json:"root_cause"`
		Disposition string `json:"disposition"`
		Evidence    string `json:"evidence"`
	}
	if err := decode(r, &x); err != nil {
		fail(w, err)
		return
	}
	trial, err := s.app.RemediateDeviation(id, dev, x.RootCause, x.Disposition, x.Evidence, x.ExpectedRevision, x.Actor, x.Role, x.RequestID)
	result(w, trial, err)
}

func (s *Server) GatePreview(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "GET" {
		method(w)
		return
	}
	report, err := s.app.GatePreview(id, r.URL.Query().Get("signer"))
	if err != nil {
		fail(w, err)
		return
	}
	write(w, 200, report)
}
func (s *Server) ReleaseTrial(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "POST" {
		method(w)
		return
	}
	var x command
	if e := decode(r, &x); e != nil {
		fail(w, e)
		return
	}
	t, e := s.app.Release(id, x.ExpectedRevision, x.Actor, x.Role, x.RequestID)
	result(w, t, e)
}
func (s *Server) ArchiveTrial(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "POST" {
		method(w)
		return
	}
	var x command
	if e := decode(r, &x); e != nil {
		fail(w, e)
		return
	}
	t, e := s.app.Archive(id, x.ExpectedRevision, x.Actor, x.Role, x.RequestID)
	result(w, t, e)
}
func (s *Server) AuditTimeline(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "GET" {
		method(w)
		return
	}
	query := r.URL.Query()
	offset, err := queryInt(query.Get("offset"), 0, "offset")
	if err != nil {
		fail(w, err)
		return
	}
	limit, err := queryInt(query.Get("limit"), 50, "limit")
	if err != nil {
		fail(w, err)
		return
	}
	from, err := queryTime(query.Get("from"), "from")
	if err != nil {
		fail(w, err)
		return
	}
	to, err := queryTime(query.Get("to"), "to")
	if err != nil {
		fail(w, err)
		return
	}
	page, err := s.app.QueryTimeline(id, application.AuditFilter{Type: query.Get("type"), Actor: query.Get("actor"), From: from, To: to, Offset: offset, Limit: limit})
	if err != nil {
		fail(w, err)
		return
	}
	write(w, 200, page)
}

func queryTime(raw, field string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, errors.New(field + " 必须为 RFC3339 时间")
	}
	return value, nil
}
func (s *Server) TrialView(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "GET" {
		method(w)
		return
	}
	view, err := s.app.View(id)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, 200, view)
}
func (s *Server) ArchiveChecklist(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "GET" {
		method(w)
		return
	}
	trial, err := s.app.Get(id)
	if err != nil {
		fail(w, err)
		return
	}
	if trial.Status != domain.Released && trial.Status != domain.Archived {
		fail(w, errors.New("仅已放行批次可查询封存清单"))
		return
	}
	write(w, 200, trial.Checklist())
}

func (s *Server) ArchiveIntegrity(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "GET" {
		method(w)
		return
	}
	proof, err := s.app.ArchiveProof(id)
	if err != nil {
		fail(w, err)
		return
	}
	write(w, 200, proof)
}
func result(w http.ResponseWriter, t *domain.VigorTrial, e error) {
	if e != nil {
		fail(w, e)
		return
	}
	write(w, 200, t)
}
func method(w http.ResponseWriter) {
	write(w, 405, map[string]any{"error": "METHOD_NOT_ALLOWED", "message": "请求方法不允许"})
}
func fail(w http.ResponseWriter, e error) {
	code := 400
	key := "VALIDATION_ERROR"
	if errors.Is(e, domain.ErrConflict) {
		code = 409
		key = "REVISION_CONFLICT"
	} else if errors.Is(e, os.ErrNotExist) {
		code = 404
		key = "NOT_FOUND"
	} else if errors.Is(e, domain.ErrInvalidTransition) {
		code = 422
		key = "INVALID_TRANSITION"
	}
	payload := map[string]any{"error": key, "message": e.Error()}
	var validation *domain.ValidationError
	if errors.As(e, &validation) {
		payload["fields"] = validation.Issues
	}
	write(w, code, payload)
}
func write(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
