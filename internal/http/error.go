package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type ErrorCode string

const (
	ErrBadRequest         ErrorCode = "BAD_REQUEST"
	ErrUnauthorized       ErrorCode = "UNAUTHORIZED"
	ErrForbidden          ErrorCode = "FORBIDDEN"
	ErrNotFound           ErrorCode = "NOT_FOUND"
	ErrConflict           ErrorCode = "CONFLICT"
	ErrValidation         ErrorCode = "VALIDATION_ERROR"
	ErrInternalServer     ErrorCode = "INTERNAL_SERVER_ERROR"
	ErrServiceUnavailable ErrorCode = "SERVICE_UNAVAILABLE"
)

type AppError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Details any       `json:"details,omitempty"`
}

func NewAppError(code ErrorCode, message string, details any) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Details: details,
	}
}

func (e *AppError) Error() string {
	return string(e.Code) + ": " + e.Message
}

func HandleError(c *gin.Context, err error) {
	if appErr, ok := err.(*AppError); ok {
		statusCode := getStatusCode(appErr.Code)
		ErrorResponse(c, statusCode, string(appErr.Code), appErr.Message)
		return
	}

	ErrorResponse(c, http.StatusInternalServerError, string(ErrInternalServer), "서버 내부 오류가 발생했습니다")
}

func getStatusCode(code ErrorCode) int {
	switch code {
	case ErrBadRequest, ErrValidation:
		return http.StatusBadRequest
	case ErrUnauthorized:
		return http.StatusUnauthorized
	case ErrForbidden:
		return http.StatusForbidden
	case ErrNotFound:
		return http.StatusNotFound
	case ErrConflict:
		return http.StatusConflict
	case ErrServiceUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}
