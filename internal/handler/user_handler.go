package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"satvos/internal/middleware"
	"satvos/internal/service"
)

// UserHandler handles user management endpoints.
type UserHandler struct {
	userService service.UserService
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(userService service.UserService) *UserHandler {
	return &UserHandler{userService: userService}
}

// Create handles POST /api/v1/users
// @Summary Create a user
// @Description Create a new user in the tenant (admin only)
// @Tags users
// @Accept json
// @Produce json
// @Param request body CreateUserRequest true "User details"
// @Success 201 {object} Response{data=domain.User} "User created"
// @Failure 400 {object} ErrorResponseBody "Validation error"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Forbidden - admin only"
// @Failure 409 {object} ErrorResponseBody "Email already exists"
// @Security BearerAuth
// @Router /users [post]
func (h *UserHandler) Create(c *gin.Context) {
	tenantID, err := middleware.GetTenantID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing tenant context")
		return
	}

	var input service.CreateUserInput
	if err := c.ShouldBindJSON(&input); err != nil {
		RespondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	user, err := h.userService.Create(c.Request.Context(), tenantID, input)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondCreated(c, user)
}

// List handles GET /api/v1/users
// @Summary List users
// @Description List all users in the tenant (admin only)
// @Tags users
// @Produce json
// @Param offset query int false "Offset for pagination" default(0)
// @Param limit query int false "Limit for pagination (max 100)" default(20)
// @Success 200 {object} Response{data=[]domain.User,meta=PagMeta} "List of users"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Forbidden - admin only"
// @Security BearerAuth
// @Router /users [get]
func (h *UserHandler) List(c *gin.Context) {
	tenantID, err := middleware.GetTenantID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing tenant context")
		return
	}

	offset, limit := parsePagination(c)

	users, total, err := h.userService.List(c.Request.Context(), tenantID, offset, limit)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondPaginated(c, users, PagMeta{Total: total, Offset: offset, Limit: limit})
}

// GetByID handles GET /api/v1/users/:id
// @Summary Get user by ID
// @Description Get user details (self or admin access)
// @Tags users
// @Produce json
// @Param id path string true "User ID (UUID)"
// @Success 200 {object} Response{data=domain.User} "User details"
// @Failure 400 {object} ErrorResponseBody "Invalid ID"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Forbidden"
// @Failure 404 {object} ErrorResponseBody "User not found"
// @Security BearerAuth
// @Router /users/{id} [get]
func (h *UserHandler) GetByID(c *gin.Context) {
	tenantID, err := middleware.GetTenantID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing tenant context")
		return
	}

	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid user ID")
		return
	}

	// Allow self-access or admin access
	currentUserID, _ := middleware.GetUserID(c)
	currentRole := middleware.GetRole(c)
	if currentUserID != userID && currentRole != "admin" {
		RespondError(c, http.StatusForbidden, "FORBIDDEN", "insufficient permissions")
		return
	}

	user, err := h.userService.GetByID(c.Request.Context(), tenantID, userID)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, user)
}

// Update handles PUT /api/v1/users/:id
// @Summary Update a user
// @Description Update user details (self can update name/email, admin can update role/active status)
// @Tags users
// @Accept json
// @Produce json
// @Param id path string true "User ID (UUID)"
// @Param request body UpdateUserRequest true "Fields to update"
// @Success 200 {object} Response{data=domain.User} "User updated"
// @Failure 400 {object} ErrorResponseBody "Validation error"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Forbidden"
// @Failure 404 {object} ErrorResponseBody "User not found"
// @Security BearerAuth
// @Router /users/{id} [put]
func (h *UserHandler) Update(c *gin.Context) {
	tenantID, err := middleware.GetTenantID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing tenant context")
		return
	}

	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid user ID")
		return
	}

	// Allow self-access or admin access
	currentUserID, _ := middleware.GetUserID(c)
	currentRole := middleware.GetRole(c)
	if currentUserID != userID && currentRole != "admin" {
		RespondError(c, http.StatusForbidden, "FORBIDDEN", "insufficient permissions")
		return
	}

	var input service.UpdateUserInput
	if err := c.ShouldBindJSON(&input); err != nil {
		RespondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	// Non-admins cannot change their own role
	if currentRole != "admin" && input.Role != nil {
		RespondError(c, http.StatusForbidden, "FORBIDDEN", "only admins can change user roles")
		return
	}

	user, err := h.userService.Update(c.Request.Context(), tenantID, userID, input)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, user)
}

// Delete handles DELETE /api/v1/users/:id
// @Summary Delete a user
// @Description Delete a user from the tenant (admin only)
// @Tags users
// @Produce json
// @Param id path string true "User ID (UUID)"
// @Success 200 {object} Response{data=MessageResponse} "User deleted"
// @Failure 400 {object} ErrorResponseBody "Invalid ID"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 403 {object} ErrorResponseBody "Forbidden - admin only"
// @Failure 404 {object} ErrorResponseBody "User not found"
// @Security BearerAuth
// @Router /users/{id} [delete]
func (h *UserHandler) Delete(c *gin.Context) {
	tenantID, err := middleware.GetTenantID(c)
	if err != nil {
		RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing tenant context")
		return
	}

	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid user ID")
		return
	}

	if err := h.userService.Delete(c.Request.Context(), tenantID, userID); err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, gin.H{"message": "user deleted"})
}
