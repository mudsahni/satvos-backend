package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"satvos/internal/service"
)

// EmailConfigHandler handles email processing configuration endpoints.
type EmailConfigHandler struct {
	emailConfigSvc service.EmailConfigService // nil = feature disabled
}

// NewEmailConfigHandler creates a new EmailConfigHandler.
func NewEmailConfigHandler(emailConfigSvc service.EmailConfigService) *EmailConfigHandler {
	return &EmailConfigHandler{emailConfigSvc: emailConfigSvc}
}

// GetConfig returns the tenant's email processing configuration.
func (h *EmailConfigHandler) GetConfig(c *gin.Context) {
	if h.emailConfigSvc == nil {
		RespondError(c, http.StatusNotFound, "NOT_AVAILABLE", "email processing configuration not available")
		return
	}

	tenantID, _, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	cfg, err := h.emailConfigSvc.GetConfig(c.Request.Context(), tenantID)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, cfg)
}

// UpdateConfig updates the tenant's email processing configuration.
func (h *EmailConfigHandler) UpdateConfig(c *gin.Context) {
	if h.emailConfigSvc == nil {
		RespondError(c, http.StatusNotFound, "NOT_AVAILABLE", "email processing configuration not available")
		return
	}

	tenantID, userID, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	var input service.UpdateEmailConfigInput
	if err := c.ShouldBindJSON(&input); err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}

	if input.Enabled == nil && input.AllowedSenders == nil {
		RespondError(c, http.StatusBadRequest, "NO_FIELDS", "at least one of 'enabled' or 'allowed_senders' must be provided")
		return
	}

	cfg, err := h.emailConfigSvc.UpdateConfig(c.Request.Context(), tenantID, userID, input)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, cfg)
}
