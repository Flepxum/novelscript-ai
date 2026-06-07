package service

import (
	"fmt"
	"net/http"

	"github.com/Flepxum/novelscript-ai/backend/internal/domain"
)

type AppError struct {
	Code    string                   `json:"code"`
	Message string                   `json:"message"`
	Details []domain.ValidationIssue `json:"details,omitempty"`
	Status  int                      `json:"-"`
}

func (e AppError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func badRequest(message string) AppError {
	return AppError{Code: "bad_request", Message: message, Status: http.StatusBadRequest}
}

func notFound(message string) AppError {
	return AppError{Code: "not_found", Message: message, Status: http.StatusNotFound}
}

func validationFailed(message string, details []domain.ValidationIssue) AppError {
	return AppError{Code: "validation_failed", Message: message, Details: details, Status: http.StatusUnprocessableEntity}
}

func internalError(message string) AppError {
	return AppError{Code: "internal_error", Message: message, Status: http.StatusInternalServerError}
}
