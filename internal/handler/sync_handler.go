package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"satvos/internal/domain"
	"satvos/internal/service"
)

// SyncHandler handles connector agent sync endpoints.
type SyncHandler struct {
	syncSvc service.SyncService
}

// NewSyncHandler creates a new SyncHandler.
func NewSyncHandler(syncSvc service.SyncService) *SyncHandler {
	return &SyncHandler{syncSvc: syncSvc}
}

// Register handles POST /sync/v1/register
func (h *SyncHandler) Register(c *gin.Context) {
	tenantID, userID, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	var req struct {
		Version string `json:"version" binding:"required"`
		OSInfo  string `json:"os_info"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	agent, err := h.syncSvc.Register(c.Request.Context(), tenantID, userID, req.Version, req.OSInfo)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondCreated(c, agent)
}

// Heartbeat handles POST /sync/v1/heartbeat
func (h *SyncHandler) Heartbeat(c *gin.Context) {
	tenantID, userID, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	var req struct {
		TallyConnected bool     `json:"tally_connected"`
		TallyCompany   string   `json:"tally_company"`
		TallyPort      int      `json:"tally_port"`
		Version        string   `json:"version"`
		Errors         []string `json:"errors"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	if err := h.syncSvc.Heartbeat(c.Request.Context(), tenantID, userID, req.TallyConnected, req.TallyCompany, req.TallyPort, req.Version, req.Errors); err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, gin.H{"status": "ok"})
}

// Masters handles POST /sync/v1/masters
func (h *SyncHandler) Masters(c *gin.Context) {
	tenantID, _, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	var payload service.MasterPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		RespondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	if err := h.syncSvc.SaveMasters(c.Request.Context(), tenantID, &payload); err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, gin.H{"message": "masters saved"})
}

// Outbound handles GET /sync/v1/outbound
func (h *SyncHandler) Outbound(c *gin.Context) {
	tenantID, _, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	cursor := c.Query("cursor")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit < 1 || limit > 50 {
		limit = 50
	}

	items, nextCursor, err := h.syncSvc.ListOutbound(c.Request.Context(), tenantID, cursor, limit)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, gin.H{"items": items, "next_cursor": nextCursor})
}

// Ack handles POST /sync/v1/ack
func (h *SyncHandler) Ack(c *gin.Context) {
	tenantID, userID, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	var req struct {
		Results []service.AckResult `json:"results" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	if err := h.syncSvc.AckOutbound(c.Request.Context(), tenantID, userID, req.Results); err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, gin.H{"message": "acknowledged"})
}

// Inbound handles POST /sync/v1/inbound
func (h *SyncHandler) Inbound(c *gin.Context) {
	tenantID, _, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	var req struct {
		Vouchers []domain.TallyVoucher `json:"vouchers" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	if err := h.syncSvc.SaveInbound(c.Request.Context(), tenantID, req.Vouchers); err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, gin.H{"message": "vouchers saved"})
}
