package application

import (
	"errors"
	"fmt"

	"radio-observation-release-gate/internal/domain"
	"radio-observation-release-gate/internal/storage"
)

type Error struct {
	Code            string `json:"code"`
	Message         string `json:"message"`
	CurrentRevision int    `json:"current_revision,omitempty"`
	HTTPStatus      int    `json:"-"`
	Details         any    `json:"details,omitempty"`
}

func (e *Error) Error() string { return e.Message }

func mapError(err error) error {
	if err == nil {
		return nil
	}
	var rule *domain.RuleError
	if errors.As(err, &rule) {
		return &Error{Code: rule.Code, Message: rule.Message, HTTPStatus: 422}
	}
	switch {
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, storage.ErrNotFound):
		return &Error{Code: "not_found", Message: "请求的资源不存在", HTTPStatus: 404}
	case errors.Is(err, domain.ErrTerminal):
		return &Error{Code: "terminal_read_only", Message: "批次已进入终态，只允许读取", HTTPStatus: 409}
	case errors.Is(err, domain.ErrInvalidState):
		return &Error{Code: "invalid_state", Message: "当前批次状态不允许此操作", HTTPStatus: 409}
	case errors.Is(err, domain.ErrConflict), errors.Is(err, storage.ErrManifestExists):
		return &Error{Code: "conflict", Message: err.Error(), HTTPStatus: 409}
	default:
		return fmt.Errorf("应用服务失败: %w", err)
	}
}
