package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"satvos/internal/domain"
	"satvos/internal/service"
)

// ReportHandler handles report endpoints.
type ReportHandler struct {
	reportService service.ReportService
}

// NewReportHandler creates a new ReportHandler.
func NewReportHandler(reportService service.ReportService) *ReportHandler {
	return &ReportHandler{reportService: reportService}
}

// validGranularities defines the allowed granularity values.
var validGranularities = map[string]bool{
	"daily":     true,
	"weekly":    true,
	"monthly":   true,
	"quarterly": true,
	"yearly":    true,
}

// parseReportFilters extracts common report filter parameters from query params.
func parseReportFilters(c *gin.Context) (*domain.ReportFilters, error) {
	filters := &domain.ReportFilters{
		Offset: 0,
		Limit:  20,
	}

	// Parse date filters.
	if fromStr := c.Query("from"); fromStr != "" {
		t, err := time.Parse("2006-01-02", fromStr)
		if err != nil {
			return nil, fmt.Errorf("invalid 'from' date: must be YYYY-MM-DD")
		}
		filters.From = &t
	}
	if toStr := c.Query("to"); toStr != "" {
		t, err := time.Parse("2006-01-02", toStr)
		if err != nil {
			return nil, fmt.Errorf("invalid 'to' date: must be YYYY-MM-DD")
		}
		filters.To = &t
	}

	// Parse collection_id filter.
	if cidStr := c.Query("collection_id"); cidStr != "" {
		cid, err := uuid.Parse(cidStr)
		if err != nil {
			return nil, fmt.Errorf("invalid 'collection_id': must be a valid UUID")
		}
		filters.CollectionID = &cid
	}

	// Parse GSTIN filters.
	filters.SellerGSTIN = c.Query("seller_gstin")
	filters.BuyerGSTIN = c.Query("buyer_gstin")

	// Parse granularity with default.
	granularity := c.Query("granularity")
	if granularity == "" {
		granularity = "monthly"
	}
	if !validGranularities[granularity] {
		return nil, fmt.Errorf("invalid 'granularity': must be one of daily, weekly, monthly, quarterly, yearly")
	}
	filters.Granularity = granularity

	// Parse pagination.
	if offsetStr := c.Query("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil {
			return nil, fmt.Errorf("invalid 'offset': must be an integer")
		}
		if offset < 0 {
			offset = 0
		}
		filters.Offset = offset
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return nil, fmt.Errorf("invalid 'limit': must be an integer")
		}
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		filters.Limit = limit
	}

	// Inject auth context.
	_, userID, role, ok := extractAuthContext(c)
	if !ok {
		return nil, fmt.Errorf("missing auth context")
	}
	filters.UserID = userID
	filters.UserRole = role

	return filters, nil
}

// Sellers handles GET /api/v1/reports/sellers
// @Summary      Seller summary report
// @Description  Lists unique sellers with invoice count, total spend, tax breakdown
// @Tags         reports
// @Produce      json
// @Param        from query string false "Start date (YYYY-MM-DD)"
// @Param        to query string false "End date (YYYY-MM-DD)"
// @Param        collection_id query string false "Collection UUID"
// @Param        buyer_gstin query string false "Filter by buyer GSTIN"
// @Param        offset query int false "Pagination offset" default(0)
// @Param        limit query int false "Pagination limit" default(20)
// @Success      200 {object} APIResponse{data=[]domain.SellerSummaryRow,meta=PagMeta}
// @Failure      400 {object} APIResponse
// @Failure      401 {object} APIResponse
// @Failure      500 {object} APIResponse
// @Security     BearerAuth
// @Router       /reports/sellers [get]
func (h *ReportHandler) Sellers(c *gin.Context) {
	tenantID, _, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	filters, err := parseReportFilters(c)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	rows, total, err := h.reportService.SellerSummary(c.Request.Context(), tenantID, filters)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondPaginated(c, rows, PagMeta{Total: total, Offset: filters.Offset, Limit: filters.Limit})
}

// Buyers handles GET /api/v1/reports/buyers
// @Summary      Buyer summary report
// @Description  Lists unique buyers with invoice count, total revenue, tax breakdown
// @Tags         reports
// @Produce      json
// @Param        from query string false "Start date (YYYY-MM-DD)"
// @Param        to query string false "End date (YYYY-MM-DD)"
// @Param        collection_id query string false "Collection UUID"
// @Param        seller_gstin query string false "Filter by seller GSTIN"
// @Param        offset query int false "Pagination offset" default(0)
// @Param        limit query int false "Pagination limit" default(20)
// @Success      200 {object} APIResponse{data=[]domain.BuyerSummaryRow,meta=PagMeta}
// @Failure      400 {object} APIResponse
// @Failure      401 {object} APIResponse
// @Failure      500 {object} APIResponse
// @Security     BearerAuth
// @Router       /reports/buyers [get]
func (h *ReportHandler) Buyers(c *gin.Context) {
	tenantID, _, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	filters, err := parseReportFilters(c)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	rows, total, err := h.reportService.BuyerSummary(c.Request.Context(), tenantID, filters)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondPaginated(c, rows, PagMeta{Total: total, Offset: filters.Offset, Limit: filters.Limit})
}

// PartyLedger handles GET /api/v1/reports/party-ledger
// @Summary      Party ledger report
// @Description  Lists all invoices for a specific GSTIN (as seller or buyer)
// @Tags         reports
// @Produce      json
// @Param        gstin query string true "Party GSTIN (required)"
// @Param        from query string false "Start date (YYYY-MM-DD)"
// @Param        to query string false "End date (YYYY-MM-DD)"
// @Param        collection_id query string false "Collection UUID"
// @Param        offset query int false "Pagination offset" default(0)
// @Param        limit query int false "Pagination limit" default(20)
// @Success      200 {object} APIResponse{data=[]domain.PartyLedgerRow,meta=PagMeta}
// @Failure      400 {object} APIResponse
// @Failure      401 {object} APIResponse
// @Failure      500 {object} APIResponse
// @Security     BearerAuth
// @Router       /reports/party-ledger [get]
func (h *ReportHandler) PartyLedger(c *gin.Context) {
	tenantID, _, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	gstin := c.Query("gstin")
	if gstin == "" {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", "gstin query parameter is required")
		return
	}

	filters, err := parseReportFilters(c)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	rows, total, err := h.reportService.PartyLedger(c.Request.Context(), tenantID, gstin, filters)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondPaginated(c, rows, PagMeta{Total: total, Offset: filters.Offset, Limit: filters.Limit})
}

// FinancialSummary handles GET /api/v1/reports/financial-summary
// @Summary      Financial summary report
// @Description  Time-series financial summary with totals per period
// @Tags         reports
// @Produce      json
// @Param        from query string false "Start date (YYYY-MM-DD)"
// @Param        to query string false "End date (YYYY-MM-DD)"
// @Param        collection_id query string false "Collection UUID"
// @Param        seller_gstin query string false "Filter by seller GSTIN"
// @Param        buyer_gstin query string false "Filter by buyer GSTIN"
// @Param        granularity query string false "Time granularity" Enums(daily, weekly, monthly, quarterly, yearly) default(monthly)
// @Success      200 {object} APIResponse{data=[]domain.FinancialSummaryRow}
// @Failure      400 {object} APIResponse
// @Failure      401 {object} APIResponse
// @Failure      500 {object} APIResponse
// @Security     BearerAuth
// @Router       /reports/financial-summary [get]
func (h *ReportHandler) FinancialSummary(c *gin.Context) {
	tenantID, _, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	filters, err := parseReportFilters(c)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	rows, err := h.reportService.FinancialSummary(c.Request.Context(), tenantID, filters)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, rows)
}

// TaxSummary handles GET /api/v1/reports/tax-summary
// @Summary      Tax summary report
// @Description  Time-series tax breakdown (intrastate CGST/SGST vs interstate IGST) per period
// @Tags         reports
// @Produce      json
// @Param        from query string false "Start date (YYYY-MM-DD)"
// @Param        to query string false "End date (YYYY-MM-DD)"
// @Param        collection_id query string false "Collection UUID"
// @Param        seller_gstin query string false "Filter by seller GSTIN"
// @Param        buyer_gstin query string false "Filter by buyer GSTIN"
// @Param        granularity query string false "Time granularity" Enums(daily, weekly, monthly, quarterly, yearly) default(monthly)
// @Success      200 {object} APIResponse{data=[]domain.TaxSummaryRow}
// @Failure      400 {object} APIResponse
// @Failure      401 {object} APIResponse
// @Failure      500 {object} APIResponse
// @Security     BearerAuth
// @Router       /reports/tax-summary [get]
func (h *ReportHandler) TaxSummary(c *gin.Context) {
	tenantID, _, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	filters, err := parseReportFilters(c)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	rows, err := h.reportService.TaxSummary(c.Request.Context(), tenantID, filters)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, rows)
}

// HSNSummary handles GET /api/v1/reports/hsn-summary
// @Summary      HSN summary report
// @Description  Aggregated HSN-code-level breakdown with quantities, taxable amounts, and tax
// @Tags         reports
// @Produce      json
// @Param        from query string false "Start date (YYYY-MM-DD)"
// @Param        to query string false "End date (YYYY-MM-DD)"
// @Param        collection_id query string false "Collection UUID"
// @Param        seller_gstin query string false "Filter by seller GSTIN"
// @Param        buyer_gstin query string false "Filter by buyer GSTIN"
// @Param        offset query int false "Pagination offset" default(0)
// @Param        limit query int false "Pagination limit" default(20)
// @Success      200 {object} APIResponse{data=[]domain.HSNSummaryRow,meta=PagMeta}
// @Failure      400 {object} APIResponse
// @Failure      401 {object} APIResponse
// @Failure      500 {object} APIResponse
// @Security     BearerAuth
// @Router       /reports/hsn-summary [get]
func (h *ReportHandler) HSNSummary(c *gin.Context) {
	tenantID, _, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	filters, err := parseReportFilters(c)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	rows, total, err := h.reportService.HSNSummary(c.Request.Context(), tenantID, filters)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondPaginated(c, rows, PagMeta{Total: total, Offset: filters.Offset, Limit: filters.Limit})
}

// CollectionsOverview handles GET /api/v1/reports/collections-overview
// @Summary      Collections overview report
// @Description  Per-collection summary with document counts, totals, and validation/review percentages
// @Tags         reports
// @Produce      json
// @Param        from query string false "Start date (YYYY-MM-DD)"
// @Param        to query string false "End date (YYYY-MM-DD)"
// @Param        collection_id query string false "Collection UUID"
// @Param        seller_gstin query string false "Filter by seller GSTIN"
// @Param        buyer_gstin query string false "Filter by buyer GSTIN"
// @Success      200 {object} APIResponse{data=[]domain.CollectionOverviewRow}
// @Failure      400 {object} APIResponse
// @Failure      401 {object} APIResponse
// @Failure      500 {object} APIResponse
// @Security     BearerAuth
// @Router       /reports/collections-overview [get]
func (h *ReportHandler) CollectionsOverview(c *gin.Context) {
	tenantID, _, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	filters, err := parseReportFilters(c)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	rows, err := h.reportService.CollectionsOverview(c.Request.Context(), tenantID, filters)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, rows)
}
