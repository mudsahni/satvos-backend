package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"satvos/internal/domain"
	"satvos/internal/middleware"
	"satvos/internal/service"
)

// ServiceAccountHandler handles service account management endpoints.
type ServiceAccountHandler struct {
	saSvc service.ServiceAccountService
}

// NewServiceAccountHandler creates a new ServiceAccountHandler.
func NewServiceAccountHandler(saSvc service.ServiceAccountService) *ServiceAccountHandler {
	return &ServiceAccountHandler{saSvc: saSvc}
}

// Create handles POST /api/v1/service-accounts
func (h *ServiceAccountHandler) Create(c *gin.Context) {
	tenantID, err := middleware.GetTenantID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing tenant context")
		return
	}
	callerID, err := middleware.GetUserID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing user context")
		return
	}

	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
		ExpiresAt   *string `json:"expires_at"` // RFC3339
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	input := &service.CreateServiceAccountInput{
		TenantID:    tenantID,
		Name:        req.Name,
		Description: req.Description,
		CreatedBy:   callerID,
	}

	if req.ExpiresAt != nil {
		t, parseErr := parseTime(*req.ExpiresAt)
		if parseErr != nil {
			RespondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "expires_at must be a valid RFC3339 timestamp")
			return
		}
		input.ExpiresAt = &t
	}

	output, err := h.saSvc.Create(c.Request.Context(), input)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondCreated(c, output)
}

// List handles GET /api/v1/service-accounts
func (h *ServiceAccountHandler) List(c *gin.Context) {
	tenantID, err := middleware.GetTenantID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing tenant context")
		return
	}

	offset, limit := parsePagination(c)

	accounts, total, err := h.saSvc.List(c.Request.Context(), tenantID, offset, limit)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondPaginated(c, accounts, PagMeta{Total: total, Offset: offset, Limit: limit})
}

// GetByID handles GET /api/v1/service-accounts/:id
func (h *ServiceAccountHandler) GetByID(c *gin.Context) {
	tenantID, err := middleware.GetTenantID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing tenant context")
		return
	}

	saID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid service account ID")
		return
	}

	sa, err := h.saSvc.GetByID(c.Request.Context(), tenantID, saID)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, sa)
}

// RotateKey handles POST /api/v1/service-accounts/:id/rotate-key
func (h *ServiceAccountHandler) RotateKey(c *gin.Context) {
	tenantID, err := middleware.GetTenantID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing tenant context")
		return
	}

	saID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid service account ID")
		return
	}

	output, err := h.saSvc.RotateAPIKey(c.Request.Context(), tenantID, saID)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, output)
}

// Revoke handles POST /api/v1/service-accounts/:id/revoke
func (h *ServiceAccountHandler) Revoke(c *gin.Context) {
	tenantID, err := middleware.GetTenantID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing tenant context")
		return
	}

	saID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid service account ID")
		return
	}

	if err := h.saSvc.Revoke(c.Request.Context(), tenantID, saID); err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, gin.H{"message": "service account revoked"})
}

// Delete handles DELETE /api/v1/service-accounts/:id
func (h *ServiceAccountHandler) Delete(c *gin.Context) {
	tenantID, err := middleware.GetTenantID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing tenant context")
		return
	}

	saID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid service account ID")
		return
	}

	if err := h.saSvc.Delete(c.Request.Context(), tenantID, saID); err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, gin.H{"message": "service account deleted"})
}

// SetPermission handles POST /api/v1/service-accounts/:id/permissions
func (h *ServiceAccountHandler) SetPermission(c *gin.Context) {
	tenantID, err := middleware.GetTenantID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing tenant context")
		return
	}
	callerID, err := middleware.GetUserID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing user context")
		return
	}

	saID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid service account ID")
		return
	}

	var req struct {
		CollectionID uuid.UUID                  `json:"collection_id" binding:"required"`
		Permission   domain.CollectionPermission `json:"permission" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	if !domain.ValidCollectionPermissions[req.Permission] {
		RespondError(c, http.StatusBadRequest, "INVALID_PERMISSION", "permission must be owner, editor, or viewer")
		return
	}

	err = h.saSvc.SetPermission(c.Request.Context(), &service.SASetPermissionInput{
		TenantID:         tenantID,
		ServiceAccountID: saID,
		CollectionID:     req.CollectionID,
		Permission:       req.Permission,
		GrantedBy:        callerID,
	})
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, gin.H{"message": "permission set"})
}

// ListPermissions handles GET /api/v1/service-accounts/:id/permissions
func (h *ServiceAccountHandler) ListPermissions(c *gin.Context) {
	tenantID, err := middleware.GetTenantID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing tenant context")
		return
	}

	saID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid service account ID")
		return
	}

	perms, err := h.saSvc.ListPermissions(c.Request.Context(), tenantID, saID)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, perms)
}

// RemovePermission handles DELETE /api/v1/service-accounts/:id/permissions/:collectionId
func (h *ServiceAccountHandler) RemovePermission(c *gin.Context) {
	tenantID, err := middleware.GetTenantID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing tenant context")
		return
	}

	saID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid service account ID")
		return
	}

	collectionID, err := uuid.Parse(c.Param("collectionId"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid collection ID")
		return
	}

	if err := h.saSvc.RemovePermission(c.Request.Context(), tenantID, saID, collectionID); err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, gin.H{"message": "permission removed"})
}

// parseTime parses an RFC3339 timestamp string.
func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}
