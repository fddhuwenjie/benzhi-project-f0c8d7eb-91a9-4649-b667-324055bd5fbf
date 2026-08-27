package domain

import "errors"

var (
	ErrInvalidInput = errors.New("输入不合法")
	ErrInvalidState = errors.New("当前状态不允许此操作")
	ErrTerminal     = errors.New("批次已进入终态，只允许读取")
	ErrNotFound     = errors.New("对象不存在")
	ErrConflict     = errors.New("资源冲突")
)

type RuleError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *RuleError) Error() string { return e.Message }

func rule(code, message string) error { return &RuleError{Code: code, Message: message} }
