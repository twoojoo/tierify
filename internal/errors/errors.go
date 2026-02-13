package errors

import (
	"fmt"
	"net/http"
)

// AppError is a structured error returned by the API.
type AppError struct {
	StatusCode int            `json:"-"`
	Code       string         `json:"error"`
	Message    string         `json:"message"`
	TenantID   string         `json:"tenant_id,omitempty"`
	Blocked    bool           `json:"blocked,omitempty"`
	Details    []LimitDetail  `json:"details,omitempty"`
}

func (e *AppError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// LimitDetail describes the state of a single limit in an error response.
type LimitDetail struct {
	LimitKey         string  `json:"limit_key"`
	LimitKind        string  `json:"limit_kind"`
	Exceeded         bool    `json:"exceeded"`
	CurrentValue     int64   `json:"current_value"`
	LimitValue       int64   `json:"limit_value"`
	Remaining        int64   `json:"remaining"`
	ResetAt          *string `json:"reset_at,omitempty"`
	RetryAfterSecs   *int64  `json:"retry_after_seconds,omitempty"`
}

// Common error constructors

func NotFound(resource string, id string) *AppError {
	return &AppError{
		StatusCode: http.StatusNotFound,
		Code:       "not_found",
		Message:    fmt.Sprintf("%s %q not found", resource, id),
	}
}

func BadRequest(message string) *AppError {
	return &AppError{
		StatusCode: http.StatusBadRequest,
		Code:       "bad_request",
		Message:    message,
	}
}

func Conflict(message string) *AppError {
	return &AppError{
		StatusCode: http.StatusConflict,
		Code:       "conflict",
		Message:    message,
	}
}

func LimitExceeded(tenantID string, details []LimitDetail) *AppError {
	return &AppError{
		StatusCode: http.StatusTooManyRequests,
		Code:       "limit_exceeded",
		Message:    "one or more limits exceeded",
		TenantID:   tenantID,
		Details:    details,
	}
}

func TenantBlocked(tenantID string) *AppError {
	return &AppError{
		StatusCode: http.StatusTooManyRequests,
		Code:       "tenant_blocked",
		Message:    "tenant is blocked",
		TenantID:   tenantID,
		Blocked:    true,
	}
}

func BelowZero(tenantID string, limitKey string) *AppError {
	return &AppError{
		StatusCode: http.StatusUnprocessableEntity,
		Code:       "usage_below_zero",
		Message:    fmt.Sprintf("decrement would bring usage below zero for limit %q", limitKey),
		TenantID:   tenantID,
	}
}

func TransactionConflict(txID string, tenantID string, limitKey string) *AppError {
	return &AppError{
		StatusCode: http.StatusConflict,
		Code:       "transaction_conflict",
		Message:    fmt.Sprintf("limit %q for tenant %q is locked by transaction %q", limitKey, tenantID, txID),
	}
}

func Internal(message string) *AppError {
	return &AppError{
		StatusCode: http.StatusInternalServerError,
		Code:       "internal_error",
		Message:    message,
	}
}

func DeprecatedTier(tierKey string, version int) *AppError {
	return &AppError{
		StatusCode: http.StatusUnprocessableEntity,
		Code:       "deprecated_tier",
		Message:    fmt.Sprintf("tier %q version %d is deprecated and cannot accept new tenants", tierKey, version),
	}
}
