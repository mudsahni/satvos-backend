package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"satvos/internal/service"
)

// TenantHandler handles tenant management endpoints.
type TenantHandler struct {
	tenantService service.TenantService
}

// NewTenantHandler creates a new TenantHandler.
func NewTenantHandler(tenantService service.TenantService) *TenantHandler {
	return &TenantHandler{tenantService: tenantService}
}

// Create handles POST /api/v1/admin/tenants
// @Summary Create a tenant
// @Description Create a new tenant (admin only)
// @Tags tenants
// @Accept json
// @Produce json
// @Param request body CreateTenantRequest true "Tenant details"
// @Success 201 {object} Response{data=domain.Tenant} "Tenant created"
// @Failure 400 {object} ErrorResponseBody "Validation error"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Forbidden - admin only"
// @Failure 409 {object} ErrorResponseBody "Slug already exists"
// @Security BearerAuth
// @Router /admin/tenants [post]
func (h *TenantHandler) Create(c *gin.Context) {
	var input service.CreateTenantInput
	if err := c.ShouldBindJSON(&input); err != nil {
		RespondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	tenant, err := h.tenantService.Create(c.Request.Context(), input)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondCreated(c, tenant)
}

// List handles GET /api/v1/admin/tenants
// @Summary List tenants
// @Description List all tenants (admin only)
// @Tags tenants
// @Produce json
// @Param offset query int false "Offset for pagination" default(0)
// @Param limit query int false "Limit for pagination (max 100)" default(20)
// @Success 200 {object} Response{data=[]domain.Tenant,meta=PagMeta} "List of tenants"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Forbidden - admin only"
// @Security BearerAuth
// @Router /admin/tenants [get]
func (h *TenantHandler) List(c *gin.Context) {
	offset, limit := parsePagination(c)

	tenants, total, err := h.tenantService.List(c.Request.Context(), offset, limit)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondPaginated(c, tenants, PagMeta{Total: total, Offset: offset, Limit: limit})
}

// GetByID handles GET /api/v1/admin/tenants/:id
// @Summary Get tenant by ID
// @Description Get tenant details (admin only)
// @Tags tenants
// @Produce json
// @Param id path string true "Tenant ID (UUID)"
// @Success 200 {object} Response{data=domain.Tenant} "Tenant details"
// @Failure 400 {object} ErrorResponseBody "Invalid ID"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Forbidden - admin only"
// @Failure 404 {object} ErrorResponseBody "Tenant not found"
// @Security BearerAuth
// @Router /admin/tenants/{id} [get]
func (h *TenantHandler) GetByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid tenant ID")
		return
	}

	tenant, err := h.tenantService.GetByID(c.Request.Context(), id)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, tenant)
}

// Update handles PUT /api/v1/admin/tenants/:id
// @Summary Update a tenant
// @Description Update tenant details (admin only)
// @Tags tenants
// @Accept json
// @Produce json
// @Param id path string true "Tenant ID (UUID)"
// @Param request body UpdateTenantRequest true "Fields to update"
// @Success 200 {object} Response{data=domain.Tenant} "Tenant updated"
// @Failure 400 {object} ErrorResponseBody "Validation error"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Forbidden - admin only"
// @Failure 404 {object} ErrorResponseBody "Tenant not found"
// @Security BearerAuth
// @Router /admin/tenants/{id} [put]
func (h *TenantHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid tenant ID")
		return
	}

	var input service.UpdateTenantInput
	if err := c.ShouldBindJSON(&input); err != nil {
		RespondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	tenant, err := h.tenantService.Update(c.Request.Context(), id, input)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, tenant)
}

// Delete handles DELETE /api/v1/admin/tenants/:id
// @Summary Delete a tenant
// @Description Delete a tenant (admin only)
// @Tags tenants
// @Produce json
// @Param id path string true "Tenant ID (UUID)"
// @Success 200 {object} Response{data=MessageResponse} "Tenant deleted"
// @Failure 400 {object} ErrorResponseBody "Invalid ID"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Forbidden - admin only"
// @Failure 404 {object} ErrorResponseBody "Tenant not found"
// @Security BearerAuth
// @Router /admin/tenants/{id} [delete]
func (h *TenantHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid tenant ID")
		return
	}

	if err := h.tenantService.Delete(c.Request.Context(), id); err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, gin.H{"message": "tenant deleted"})
}
