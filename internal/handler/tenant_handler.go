package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"satvos/internal/middleware"
	"satvos/internal/service"
)

// TenantHandler handles tenant management endpoints.
// All operations are scoped to the caller's own tenant via JWT tenant_id.
type TenantHandler struct {
	tenantService service.TenantService
}

// NewTenantHandler creates a new TenantHandler.
func NewTenantHandler(tenantService service.TenantService) *TenantHandler {
	return &TenantHandler{tenantService: tenantService}
}

// GetOwnTenant handles GET /api/v1/admin/tenant
// @Summary Get own tenant
// @Description Get the caller's own tenant details (admin only)
// @Tags tenants
// @Produce json
// @Success 200 {object} Response{data=domain.Tenant} "Tenant details"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Forbidden - admin only"
// @Failure 404 {object} ErrorResponseBody "Tenant not found"
// @Security BearerAuth
// @Router /admin/tenant [get]
func (h *TenantHandler) GetOwnTenant(c *gin.Context) {
	tenantID, err := middleware.GetTenantID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing tenant context")
		return
	}

	tenant, err := h.tenantService.GetByID(c.Request.Context(), tenantID)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, tenant)
}

// UpdateOwnTenant handles PUT /api/v1/admin/tenant
// @Summary Update own tenant
// @Description Update the caller's own tenant details (admin only)
// @Tags tenants
// @Accept json
// @Produce json
// @Param request body UpdateTenantRequest true "Fields to update"
// @Success 200 {object} Response{data=domain.Tenant} "Tenant updated"
// @Failure 400 {object} ErrorResponseBody "Validation error"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Forbidden - admin only"
// @Failure 404 {object} ErrorResponseBody "Tenant not found"
// @Security BearerAuth
// @Router /admin/tenant [put]
func (h *TenantHandler) UpdateOwnTenant(c *gin.Context) {
	tenantID, err := middleware.GetTenantID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing tenant context")
		return
	}

	var input service.UpdateTenantInput
	if err := c.ShouldBindJSON(&input); err != nil {
		RespondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	// Tenant admins cannot toggle their own tenant's active status (self-lockout prevention)
	input.IsActive = nil

	tenant, err := h.tenantService.Update(c.Request.Context(), tenantID, input)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, tenant)
}

// --- Legacy endpoints (scoped to own tenant) ---

// Create handles POST /api/v1/admin/tenants
// Disabled: tenant creation is a platform-level operation not available to tenant admins.
func (h *TenantHandler) Create(c *gin.Context) {
	RespondError(c, http.StatusForbidden, "FORBIDDEN", "tenant creation is not available")
}

// List handles GET /api/v1/admin/tenants
// Returns only the caller's own tenant.
func (h *TenantHandler) List(c *gin.Context) {
	tenantID, err := middleware.GetTenantID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing tenant context")
		return
	}

	tenant, err := h.tenantService.GetByID(c.Request.Context(), tenantID)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondPaginated(c, []interface{}{tenant}, PagMeta{Total: 1, Offset: 0, Limit: 1})
}

// GetByID handles GET /api/v1/admin/tenants/:id
// Only returns data if the requested ID matches the caller's own tenant.
func (h *TenantHandler) GetByID(c *gin.Context) {
	tenantID, err := middleware.GetTenantID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing tenant context")
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid tenant ID")
		return
	}

	if id != tenantID {
		RespondError(c, http.StatusForbidden, "FORBIDDEN", "you can only view your own tenant")
		return
	}

	tenant, err := h.tenantService.GetByID(c.Request.Context(), tenantID)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, tenant)
}

// Update handles PUT /api/v1/admin/tenants/:id
// Only allows update if the requested ID matches the caller's own tenant.
func (h *TenantHandler) Update(c *gin.Context) {
	tenantID, err := middleware.GetTenantID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing tenant context")
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid tenant ID")
		return
	}

	if id != tenantID {
		RespondError(c, http.StatusForbidden, "FORBIDDEN", "you can only update your own tenant")
		return
	}

	var input service.UpdateTenantInput
	if err := c.ShouldBindJSON(&input); err != nil {
		RespondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	// Tenant admins cannot toggle their own tenant's active status (self-lockout prevention)
	input.IsActive = nil

	tenant, err := h.tenantService.Update(c.Request.Context(), tenantID, input)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, tenant)
}

// Delete handles DELETE /api/v1/admin/tenants/:id
// Disabled: tenant deletion is a platform-level operation not available to tenant admins.
func (h *TenantHandler) Delete(c *gin.Context) {
	RespondError(c, http.StatusForbidden, "FORBIDDEN", "tenant deletion is not available")
}
