package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"radio-observation-release-gate/internal/application"
)

const maxRequestBody = 1 << 20

type errorBody struct {
	Error errorDetail `json:"error"`
}
type errorDetail struct {
	Code            string `json:"code"`
	Message         string `json:"message"`
	CurrentRevision int    `json:"current_revision,omitempty"`
	Details         any    `json:"details,omitempty"`
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		return &application.Error{Code: "content_type_invalid", Message: "Content-Type 必须是 application/json", HTTPStatus: 415}
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return &application.Error{Code: "invalid_json", Message: "请求 JSON 不合法: " + err.Error(), HTTPStatus: 400}
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return &application.Error{Code: "invalid_json", Message: "请求体只能包含一个 JSON 对象", HTTPStatus: 400}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	var appErr *application.Error
	if errors.As(err, &appErr) {
		writeJSON(w, appErr.HTTPStatus, errorBody{errorDetail{Code: appErr.Code, Message: appErr.Message, CurrentRevision: appErr.CurrentRevision, Details: appErr.Details}})
		return
	}
	writeJSON(w, http.StatusInternalServerError, errorBody{Error: errorDetail{Code: "internal_error", Message: "服务处理请求失败"}})
}
