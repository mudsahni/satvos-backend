package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"satvos/internal/domain"
	"satvos/internal/middleware"
)

// APIResponse is the standard envelope for all API responses.
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *APIError   `json:"error,omitempty"`
	Meta    *PagMeta    `json:"meta,omitempty"`
}

// APIError holds error details in the response.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// PagMeta holds pagination metadata.
type PagMeta struct {
	Total  int `json:"total"`
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

// RespondOK sends a 200 success response.
func RespondOK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, APIResponse{Success: true, Data: data})
}

// RespondCreated sends a 201 success response.
func RespondCreated(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, APIResponse{Success: true, Data: data})
}

// RespondPaginated sends a 200 success response with pagination metadata.
func RespondPaginated(c *gin.Context, data interface{}, meta PagMeta) {
	c.JSON(http.StatusOK, APIResponse{Success: true, Data: data, Meta: &meta})
}

// RespondError sends an error response with the given status code.
func RespondError(c *gin.Context, status int, code, msg string) {
	c.JSON(status, APIResponse{
		Success: false,
		Error:   &APIError{Code: code, Message: msg},
	})
}

// MapDomainError translates domain errors to HTTP status codes and error codes.
func MapDomainError(err error) (status int, code, msg string) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, "NOT_FOUND", "resource not found"
	case errors.Is(err, domain.ErrUnauthorized):
		return http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized"
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden, "FORBIDDEN", "forbidden"
	case errors.Is(err, domain.ErrInvalidCredentials):
		return http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid credentials"
	case errors.Is(err, domain.ErrTenantInactive):
		return http.StatusForbidden, "TENANT_INACTIVE", "tenant is inactive"
	case errors.Is(err, domain.ErrUserInactive):
		return http.StatusForbidden, "USER_INACTIVE", "user is inactive"
	case errors.Is(err, domain.ErrUnsupportedFileType):
		return http.StatusBadRequest, "UNSUPPORTED_FILE_TYPE", "unsupported file type; allowed: pdf, jpg, png"
	case errors.Is(err, domain.ErrFileTooLarge):
		return http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "file exceeds maximum allowed size"
	case errors.Is(err, domain.ErrDuplicateEmail):
		return http.StatusConflict, "DUPLICATE_EMAIL", "email already exists for this tenant"
	case errors.Is(err, domain.ErrDuplicateTenantSlug):
		return http.StatusConflict, "DUPLICATE_SLUG", "tenant slug already exists"
	case errors.Is(err, domain.ErrUploadFailed):
		return http.StatusInternalServerError, "UPLOAD_FAILED", "file upload to storage failed"
	case errors.Is(err, domain.ErrCollectionNotFound):
		return http.StatusNotFound, "COLLECTION_NOT_FOUND", "collection not found"
	case errors.Is(err, domain.ErrCollectionPermDenied):
		return http.StatusForbidden, "COLLECTION_PERMISSION_DENIED", "insufficient collection permission"
	case errors.Is(err, domain.ErrDuplicateCollectionFile):
		return http.StatusConflict, "DUPLICATE_COLLECTION_FILE", "file already exists in collection"
	case errors.Is(err, domain.ErrSelfPermissionRemoval):
		return http.StatusBadRequest, "SELF_PERMISSION_REMOVAL", "cannot remove your own permission"
	case errors.Is(err, domain.ErrInvalidPermission):
		return http.StatusBadRequest, "INVALID_PERMISSION", "invalid collection permission; allowed: owner, editor, viewer"
	case errors.Is(err, domain.ErrDocumentNotFound):
		return http.StatusNotFound, "DOCUMENT_NOT_FOUND", "document not found"
	case errors.Is(err, domain.ErrDocumentAlreadyExists):
		return http.StatusConflict, "DOCUMENT_ALREADY_EXISTS", "document already exists for this file"
	case errors.Is(err, domain.ErrDocumentNotParsed):
		return http.StatusBadRequest, "DOCUMENT_NOT_PARSED", "document has not been parsed yet"
	case errors.Is(err, domain.ErrInsufficientRole):
		return http.StatusForbidden, "INSUFFICIENT_ROLE", "insufficient role for this action"
	case errors.Is(err, domain.ErrInvalidStructuredData):
		return http.StatusBadRequest, "INVALID_STRUCTURED_DATA", "structured data does not match expected format"
	case errors.Is(err, domain.ErrQuotaExceeded):
		return http.StatusTooManyRequests, "QUOTA_EXCEEDED", "monthly document quota exceeded; upgrade for more"
	case errors.Is(err, domain.ErrEmailNotVerified):
		return http.StatusForbidden, "EMAIL_NOT_VERIFIED", "please verify your email before performing this action"
	case errors.Is(err, domain.ErrPasswordResetTokenInvalid):
		return http.StatusUnauthorized, "INVALID_RESET_TOKEN", "password reset token is invalid or has already been used"
	case errors.Is(err, domain.ErrSocialAuthTokenInvalid):
		return http.StatusUnauthorized, "INVALID_SOCIAL_TOKEN", "social authentication token is invalid or expired"
	case errors.Is(err, domain.ErrPasswordLoginNotAllowed):
		return http.StatusBadRequest, "PASSWORD_LOGIN_NOT_ALLOWED", "this account uses social login; use your social provider to sign in"
	case errors.Is(err, domain.ErrAssigneeCannotReview):
		return http.StatusBadRequest, "ASSIGNEE_CANNOT_REVIEW", "assignee does not have review permission on this collection"
	case errors.Is(err, domain.ErrServiceAccountReview):
		return http.StatusForbidden, "SERVICE_ACCOUNT_REVIEW", "service accounts cannot review or be assigned documents"
	case errors.Is(err, domain.ErrServiceAccountNotFound):
		return http.StatusNotFound, "SERVICE_ACCOUNT_NOT_FOUND", "service account not found"
	case errors.Is(err, domain.ErrAPIKeyInvalid):
		return http.StatusUnauthorized, "INVALID_API_KEY", "invalid API key"
	case errors.Is(err, domain.ErrAPIKeyRevoked):
		return http.StatusUnauthorized, "API_KEY_REVOKED", "API key has been revoked or expired"
	case errors.Is(err, domain.ErrInvitationPending):
		return http.StatusForbidden, "INVITATION_PENDING", "please check your email and accept the invitation to set your password"
	case errors.Is(err, domain.ErrInvitationTokenInvalid):
		return http.StatusUnauthorized, "INVALID_INVITATION_TOKEN", "invitation token is invalid, expired, or has already been used"
	default:
		return http.StatusInternalServerError, "INTERNAL_ERROR", "an internal error occurred"
	}
}

// extractAuthContext extracts tenant ID, user ID, and role from the request context.
// Returns false if auth context is missing (error response already written).
func extractAuthContext(c *gin.Context) (tenantID, userID uuid.UUID, role domain.UserRole, ok bool) {
	var err error
	tenantID, err = middleware.GetTenantID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing tenant context")
		return uuid.Nil, uuid.Nil, "", false
	}
	userID, err = middleware.GetUserID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing user context")
		return uuid.Nil, uuid.Nil, "", false
	}
	role = domain.UserRole(middleware.GetRole(c))
	return tenantID, userID, role, true
}

// HandleError maps a domain error and sends the appropriate error response.
func HandleError(c *gin.Context, err error) {
	status, code, msg := MapDomainError(err)
	if status >= 500 {
		requestID, _ := c.Get("request_id")
		log.Printf("[%s] internal error: %v", requestID, err)
	}
	RespondError(c, status, code, msg)
}

// parsePagination extracts offset and limit from query params with defaults and bounds checking.
func parsePagination(c *gin.Context) (offset, limit int) {
	offset, _ = strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ = strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	return offset, limit
}
