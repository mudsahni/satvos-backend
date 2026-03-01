package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"satvos/internal/port"
)

// ConnectorHandler handles admin-facing Tally connector endpoints.
type ConnectorHandler struct {
	syncRepo    port.SyncRepository
	masterRepo  port.TallyMasterRepository
	voucherRepo port.TallyVoucherRepository
	storage     port.ObjectStorage
	s3Bucket    string
}

// NewConnectorHandler creates a new ConnectorHandler. All repos are nil-safe.
func NewConnectorHandler(
	syncRepo port.SyncRepository,
	masterRepo port.TallyMasterRepository,
	voucherRepo port.TallyVoucherRepository,
	storage port.ObjectStorage,
	s3Bucket string,
) *ConnectorHandler {
	return &ConnectorHandler{
		syncRepo:    syncRepo,
		masterRepo:  masterRepo,
		voucherRepo: voucherRepo,
		storage:     storage,
		s3Bucket:    s3Bucket,
	}
}

// ListConnectors handles GET /api/v1/admin/connectors
func (h *ConnectorHandler) ListConnectors(c *gin.Context) {
	if h.syncRepo == nil {
		RespondError(c, http.StatusNotFound, "NOT_AVAILABLE", "connector feature not available")
		return
	}

	tenantID, _, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	agents, err := h.syncRepo.ListAgents(c.Request.Context(), tenantID)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, agents)
}

// ListLedgers handles GET /api/v1/admin/tally-masters/ledgers
func (h *ConnectorHandler) ListLedgers(c *gin.Context) {
	if h.masterRepo == nil {
		RespondError(c, http.StatusNotFound, "NOT_AVAILABLE", "tally masters not available")
		return
	}

	tenantID, _, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	parentGroup := c.Query("parent_group")
	taxType := c.Query("tax_type")
	search := c.Query("search")
	offset, limit := parsePagination(c)

	ledgers, total, err := h.masterRepo.ListLedgersPaginated(c.Request.Context(), tenantID, parentGroup, taxType, search, offset, limit)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondPaginated(c, ledgers, PagMeta{Total: total, Offset: offset, Limit: limit})
}

// ListStockItems handles GET /api/v1/admin/tally-masters/stock-items
func (h *ConnectorHandler) ListStockItems(c *gin.Context) {
	if h.masterRepo == nil {
		RespondError(c, http.StatusNotFound, "NOT_AVAILABLE", "tally masters not available")
		return
	}

	tenantID, _, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	parentGroup := c.Query("parent_group")
	hsnCode := c.Query("hsn_code")
	search := c.Query("search")
	offset, limit := parsePagination(c)

	items, total, err := h.masterRepo.ListStockItemsPaginated(c.Request.Context(), tenantID, parentGroup, hsnCode, search, offset, limit)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondPaginated(c, items, PagMeta{Total: total, Offset: offset, Limit: limit})
}

// ListGodowns handles GET /api/v1/admin/tally-masters/godowns
func (h *ConnectorHandler) ListGodowns(c *gin.Context) {
	if h.masterRepo == nil {
		RespondError(c, http.StatusNotFound, "NOT_AVAILABLE", "tally masters not available")
		return
	}

	tenantID, _, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	godowns, err := h.masterRepo.ListGodowns(c.Request.Context(), tenantID)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, godowns)
}

// ListUnits handles GET /api/v1/admin/tally-masters/units
func (h *ConnectorHandler) ListUnits(c *gin.Context) {
	if h.masterRepo == nil {
		RespondError(c, http.StatusNotFound, "NOT_AVAILABLE", "tally masters not available")
		return
	}

	tenantID, _, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	units, err := h.masterRepo.ListUnits(c.Request.Context(), tenantID)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, units)
}

// ListCostCentres handles GET /api/v1/admin/tally-masters/cost-centres
func (h *ConnectorHandler) ListCostCentres(c *gin.Context) {
	if h.masterRepo == nil {
		RespondError(c, http.StatusNotFound, "NOT_AVAILABLE", "tally masters not available")
		return
	}

	tenantID, _, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	centers, err := h.masterRepo.ListCostCentres(c.Request.Context(), tenantID) //nolint:misspell // interface method uses British spelling
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, centers)
}

// ListTallyVouchers handles GET /api/v1/admin/tally-vouchers
func (h *ConnectorHandler) ListTallyVouchers(c *gin.Context) {
	if h.voucherRepo == nil {
		RespondError(c, http.StatusNotFound, "NOT_AVAILABLE", "tally vouchers not available")
		return
	}

	tenantID, _, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	voucherType := c.Query("voucher_type")
	partyGSTIN := c.Query("party_gstin")

	var from, to *time.Time
	if v := c.Query("from"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid 'from' date format, expected YYYY-MM-DD")
			return
		}
		from = &t
	}
	if v := c.Query("to"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid 'to' date format, expected YYYY-MM-DD")
			return
		}
		to = &t
	}

	offset, limit := parsePagination(c)

	vouchers, total, err := h.voucherRepo.ListByTenantFiltered(c.Request.Context(), tenantID, voucherType, partyGSTIN, from, to, offset, limit)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondPaginated(c, vouchers, PagMeta{Total: total, Offset: offset, Limit: limit})
}

// GetTallyVoucher handles GET /api/v1/admin/tally-vouchers/:id
func (h *ConnectorHandler) GetTallyVoucher(c *gin.Context) {
	if h.voucherRepo == nil {
		RespondError(c, http.StatusNotFound, "NOT_AVAILABLE", "tally vouchers not available")
		return
	}

	tenantID, _, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	voucherID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_ID", "invalid voucher ID")
		return
	}

	voucher, err := h.voucherRepo.GetByID(c.Request.Context(), tenantID, voucherID)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, voucher)
}

// DownloadConnector handles GET /api/v1/admin/connector-download
func (h *ConnectorHandler) DownloadConnector(c *gin.Context) {
	if h.storage == nil {
		RespondError(c, http.StatusNotFound, "NOT_AVAILABLE", "connector download not available")
		return
	}

	const (
		binaryKey   = "connectors/tally/latest/satvos-connector.exe"
		checksumKey = "connectors/tally/latest/satvos-connector.exe.sha256"
		filename    = "satvos-connector.exe"
		expiry      = int64(300) // 5 minutes
	)

	downloadURL, err := h.storage.GetPresignedURL(c.Request.Context(), h.s3Bucket, binaryKey, expiry)
	if err != nil {
		HandleError(c, err)
		return
	}

	checksumURL, err := h.storage.GetPresignedURL(c.Request.Context(), h.s3Bucket, checksumKey, expiry)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, gin.H{
		"download_url": downloadURL,
		"checksum_url": checksumURL,
		"filename":     filename,
	})
}
