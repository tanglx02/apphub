// Package api 提供 REST API 与静态资源服务。
package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/tanglx02/apphub/internal/buildinfo"
)

// Response 统一 JSON 响应结构。
type Response struct {
	Success   bool   `json:"success"`
	Data      any    `json:"data"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

// ErrorPayload 业务错误码（便于前端做差异化处理）。
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// 错误码常量。
const (
	CodeUnauthorized   = "UNAUTHORIZED"
	CodeForbidden      = "FORBIDDEN"
	CodeBadRequest     = "BAD_REQUEST"
	CodeNotFound       = "NOT_FOUND"
	CodeConflict       = "CONFLICT"
	CodeRateLimited    = "RATE_LIMITED"
	CodeCSRF           = "CSRF_FAILED"
	CodeInternal       = "INTERNAL_ERROR"
	CodeNotInitialized = "NOT_INITIALIZED"
)

// ErrHTTP 携带状态码与错误码的错误。
type ErrHTTP struct {
	Status  int
	Code    string
	Message string
	Err     error
}

func (e *ErrHTTP) Error() string { return e.Message }

// Unwrap 返回底层错误。
func (e *ErrHTTP) Unwrap() error { return e.Err }

// NewErr 构造 HTTP 错误。
func NewErr(status int, code, message string) *ErrHTTP {
	return &ErrHTTP{Status: status, Code: code, Message: message}
}

// WrapErr 包装底层错误为用户友好错误（真实错误写入日志，不下发）。
func WrapErr(status int, code, userMessage string, err error) *ErrHTTP {
	return &ErrHTTP{Status: status, Code: code, Message: userMessage, Err: err}
}

func requestID(r *http.Request) string {
	if v, ok := r.Context().Value(ctxKeyRequestID).(string); ok {
		return v
	}
	return ""
}

// writeJSON 输出统一响应。
func writeJSON(w http.ResponseWriter, r *http.Request, status int, data any, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Response{
		Success:   status < 400,
		Data:      data,
		Message:   message,
		RequestID: requestID(r),
	})
}

// OK 成功响应。
func OK(w http.ResponseWriter, r *http.Request, data any) {
	writeJSON(w, r, http.StatusOK, data, "")
}

// Created 创建成功响应。
func Created(w http.ResponseWriter, r *http.Request, data any) {
	writeJSON(w, r, http.StatusCreated, data, "")
}

// Fail 失败响应（用户可见信息，不含内部细节）。
func Fail(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSON(w, r, status, ErrorPayload{Code: code, Message: message}, message)
}

// FailErr 根据错误类型输出响应。
func FailErr(w http.ResponseWriter, r *http.Request, err error) {
	var he *ErrHTTP
	if errors.As(err, &he) {
		if he.Err != nil {
			logError(r, "%s: %v", he.Message, he.Err)
		}
		Fail(w, r, he.Status, he.Code, he.Message)
		return
	}
	logError(r, "内部错误: %v", err)
	Fail(w, r, http.StatusInternalServerError, CodeInternal, "服务器内部错误，请查看日志了解详情")
}

// APIVersion 对外 API 版本。
func APIVersion() string { return buildinfo.APIVersion }
