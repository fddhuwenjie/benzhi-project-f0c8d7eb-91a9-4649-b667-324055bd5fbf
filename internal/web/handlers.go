package web

import (
	"net/http"
	"strconv"
	"strings"

	"radio-observation-release-gate/internal/application"
	"radio-observation-release-gate/internal/domain"
)

func (s *Server) WorkbenchHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	raw, err := assets.ReadFile("assets/index.html")
	if err != nil {
		http.Error(w, "工作台资源不可用", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(raw)
}

func (s *Server) HealthHandler(w http.ResponseWriter, r *http.Request) {
	result := s.app.Health()
	status := http.StatusOK
	if result["status"] != "ok" {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, result)
}

func (s *Server) ListBatchesHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, pageSize := 1, 50
	var err error
	if raw := q.Get("page"); raw != "" {
		page, err = strconv.Atoi(raw)
		if err != nil || page < 1 {
			writeError(w, &application.Error{Code: "page_invalid", Message: "page 必须为正整数", HTTPStatus: 400})
			return
		}
	}
	if raw := q.Get("page_size"); raw != "" {
		pageSize, err = strconv.Atoi(raw)
		if err != nil || pageSize < 1 {
			writeError(w, &application.Error{Code: "page_size_invalid", Message: "page_size 必须为正整数", HTTPStatus: 400})
			return
		}
		if pageSize > 100 {
			writeError(w, &application.Error{Code: "page_size_exceeded", Message: "page_size 不能超过 100", HTTPStatus: 400})
			return
		}
	}
	state := domain.BatchState(strings.TrimSpace(q.Get("state")))
	if state != "" && !validBatchState(state) {
		writeError(w, &application.Error{Code: "state_invalid", Message: "未知批次状态", HTTPStatus: 400})
		return
	}
	todo := false
	if raw := q.Get("todo"); raw != "" {
		todo, err = strconv.ParseBool(raw)
		if err != nil {
			writeError(w, &application.Error{Code: "todo_invalid", Message: "todo 必须为布尔值", HTTPStatus: 400})
			return
		}
	}
	result, err := s.app.QueryBatches(application.BatchListFilter{BatchID: strings.TrimSpace(q.Get("batch_id")), TelescopeID: strings.TrimSpace(q.Get("telescope_id")), TargetSource: strings.TrimSpace(q.Get("target_source")), State: state, TodoOnly: todo, Page: page, PageSize: pageSize})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}

func validBatchState(v domain.BatchState) bool {
	switch v {
	case domain.StateDraft, domain.StateFrozen, domain.StateQuality, domain.StateRemediation, domain.StateReview, domain.StateApproved, domain.StateRejected:
		return true
	}
	return false
}

func (s *Server) CreateBatchHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.CreateBatchCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.app.CreateBatch(cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 201, result)
}

func (s *Server) GetBatchHandler(w http.ResponseWriter, r *http.Request) {
	result, err := s.app.GetBatch(r.PathValue("batchID"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) FreezeBaselineHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.FreezeBaselineCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.app.FreezeBaseline(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) RegisterSegmentHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.RegisterSegmentCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.app.RegisterSegment(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) RegisterSegmentsHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.RegisterSegmentsCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.app.RegisterSegments(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) AssessSegmentHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.AssessSegmentCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.app.AssessSegment(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) AssessBatchHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.AssessBatchCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.app.AssessBatch(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) PreviewReplacementHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.PreviewReplacementCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.app.PreviewReplacement(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) QuarantineHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.QuarantineCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.app.Quarantine(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) GenerateReviewHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.GenerateReviewCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.app.GenerateReview(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) DecideReviewHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.DecideReviewCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.app.DecideReview(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) AssignReviewHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.AssignReviewCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.app.AssignReview(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) SealHandler(w http.ResponseWriter, r *http.Request) {
	var cmd application.SealCommand
	if err := decodeJSON(w, r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.app.Seal(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) TimelineHandler(w http.ResponseWriter, r *http.Request) {
	result, err := s.app.Timeline(r.PathValue("batchID"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) VerifyManifestHandler(w http.ResponseWriter, r *http.Request) {
	result, err := s.app.VerifyManifest(r.PathValue("batchID"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
