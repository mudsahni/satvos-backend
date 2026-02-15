package handler_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
	"satvos/internal/handler"
	"satvos/mocks"
)

func newReportHandler() (*handler.ReportHandler, *mocks.MockReportService) {
	mockSvc := new(mocks.MockReportService)
	h := handler.NewReportHandler(mockSvc)
	return h, mockSvc
}

func TestReportHandler_Sellers_Success(t *testing.T) {
	h, mockSvc := newReportHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	expected := []domain.SellerSummaryRow{
		{
			SellerGSTIN:  "29AABCT1332L1ZP",
			SellerName:   "Test Seller",
			InvoiceCount: 5,
			TotalAmount:  50000,
		},
	}

	mockSvc.On("SellerSummary", mock.Anything, tenantID, mock.AnythingOfType("*domain.ReportFilters")).Return(expected, 1, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/reports/sellers", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.Sellers(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	assert.NotNil(t, resp.Meta)
	mockSvc.AssertExpectations(t)
}

func TestReportHandler_Buyers_Success(t *testing.T) {
	h, mockSvc := newReportHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	expected := []domain.BuyerSummaryRow{
		{
			BuyerGSTIN:   "27AABCU9603R1ZM",
			BuyerName:    "Test Buyer",
			InvoiceCount: 3,
			TotalAmount:  30000,
		},
	}

	mockSvc.On("BuyerSummary", mock.Anything, tenantID, mock.AnythingOfType("*domain.ReportFilters")).Return(expected, 1, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/reports/buyers", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.Buyers(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	assert.NotNil(t, resp.Meta)
	mockSvc.AssertExpectations(t)
}

func TestReportHandler_PartyLedger_Success(t *testing.T) {
	h, mockSvc := newReportHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	expected := []domain.PartyLedgerRow{
		{
			DocumentID:    uuid.New(),
			InvoiceNumber: "INV-1",
		},
	}

	mockSvc.On("PartyLedger", mock.Anything, tenantID, "29AABCT1332L1ZP", mock.AnythingOfType("*domain.ReportFilters")).Return(expected, 1, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/reports/party-ledger?gstin=29AABCT1332L1ZP", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.PartyLedger(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	assert.NotNil(t, resp.Meta)
	mockSvc.AssertExpectations(t)
}

func TestReportHandler_PartyLedger_MissingGSTIN(t *testing.T) {
	h, _ := newReportHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/reports/party-ledger", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.PartyLedger(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.False(t, resp.Success)
}

func TestReportHandler_FinancialSummary_Success(t *testing.T) {
	h, mockSvc := newReportHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	expected := []domain.FinancialSummaryRow{
		{
			Period:       "2025-04",
			InvoiceCount: 10,
		},
	}

	mockSvc.On("FinancialSummary", mock.Anything, tenantID, mock.AnythingOfType("*domain.ReportFilters")).Return(expected, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/reports/financial-summary?from=2025-04-01&to=2026-03-31", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.FinancialSummary(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Nil(t, resp.Meta) // RespondOK does not set Meta
	mockSvc.AssertExpectations(t)
}

func TestReportHandler_TaxSummary_Success(t *testing.T) {
	h, mockSvc := newReportHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	expected := []domain.TaxSummaryRow{
		{
			Period: "2025-04",
		},
	}

	mockSvc.On("TaxSummary", mock.Anything, tenantID, mock.AnythingOfType("*domain.ReportFilters")).Return(expected, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/reports/tax-summary", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.TaxSummary(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	mockSvc.AssertExpectations(t)
}

func TestReportHandler_HSNSummary_Success(t *testing.T) {
	h, mockSvc := newReportHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	expected := []domain.HSNSummaryRow{
		{
			HSNCode: "8471",
		},
	}

	mockSvc.On("HSNSummary", mock.Anything, tenantID, mock.AnythingOfType("*domain.ReportFilters")).Return(expected, 1, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/reports/hsn-summary", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.HSNSummary(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	assert.NotNil(t, resp.Meta)
	mockSvc.AssertExpectations(t)
}

func TestReportHandler_CollectionsOverview_Success(t *testing.T) {
	h, mockSvc := newReportHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	expected := []domain.CollectionOverviewRow{
		{
			CollectionID: uuid.New(),
		},
	}

	mockSvc.On("CollectionsOverview", mock.Anything, tenantID, mock.AnythingOfType("*domain.ReportFilters")).Return(expected, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/reports/collections-overview", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.CollectionsOverview(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	mockSvc.AssertExpectations(t)
}

func TestReportHandler_InvalidDateFilter(t *testing.T) {
	h, _ := newReportHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/reports/sellers?from=not-a-date", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.Sellers(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.False(t, resp.Success)
}

func TestReportHandler_InvalidGranularity(t *testing.T) {
	h, _ := newReportHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/reports/financial-summary?granularity=hourly", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.FinancialSummary(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.False(t, resp.Success)
}

func TestReportHandler_NoAuth(t *testing.T) {
	h, _ := newReportHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/reports/sellers", http.NoBody)
	// No auth context set

	h.Sellers(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestReportHandler_Sellers_ServiceError(t *testing.T) {
	h, mockSvc := newReportHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	var nilRows []domain.SellerSummaryRow
	mockSvc.On("SellerSummary", mock.Anything, tenantID, mock.AnythingOfType("*domain.ReportFilters")).Return(nilRows, 0, errors.New("db error"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/reports/sellers", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.Sellers(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestReportHandler_Sellers_InvalidCollectionID(t *testing.T) {
	h, _ := newReportHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/reports/sellers?collection_id=not-a-uuid", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.Sellers(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.False(t, resp.Success)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, "INVALID_REQUEST", resp.Error.Code)
}

func TestReportHandler_Buyers_ServiceError(t *testing.T) {
	h, mockSvc := newReportHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	var nilRows []domain.BuyerSummaryRow
	mockSvc.On("BuyerSummary", mock.Anything, tenantID, mock.AnythingOfType("*domain.ReportFilters")).Return(nilRows, 0, errors.New("db error"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/reports/buyers", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.Buyers(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestReportHandler_FinancialSummary_ServiceError(t *testing.T) {
	h, mockSvc := newReportHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	var nilRows []domain.FinancialSummaryRow
	mockSvc.On("FinancialSummary", mock.Anything, tenantID, mock.AnythingOfType("*domain.ReportFilters")).Return(nilRows, errors.New("db error"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/reports/financial-summary", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.FinancialSummary(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestReportHandler_TaxSummary_ServiceError(t *testing.T) {
	h, mockSvc := newReportHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	var nilRows []domain.TaxSummaryRow
	mockSvc.On("TaxSummary", mock.Anything, tenantID, mock.AnythingOfType("*domain.ReportFilters")).Return(nilRows, errors.New("db error"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/reports/tax-summary", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.TaxSummary(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestReportHandler_HSNSummary_ServiceError(t *testing.T) {
	h, mockSvc := newReportHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	var nilRows []domain.HSNSummaryRow
	mockSvc.On("HSNSummary", mock.Anything, tenantID, mock.AnythingOfType("*domain.ReportFilters")).Return(nilRows, 0, errors.New("db error"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/reports/hsn-summary", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.HSNSummary(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestReportHandler_CollectionsOverview_ServiceError(t *testing.T) {
	h, mockSvc := newReportHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	var nilRows []domain.CollectionOverviewRow
	mockSvc.On("CollectionsOverview", mock.Anything, tenantID, mock.AnythingOfType("*domain.ReportFilters")).Return(nilRows, errors.New("db error"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/reports/collections-overview", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.CollectionsOverview(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestReportHandler_PartyLedger_ServiceError(t *testing.T) {
	h, mockSvc := newReportHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	var nilRows []domain.PartyLedgerRow
	mockSvc.On("PartyLedger", mock.Anything, tenantID, "29AABCT1332L1ZP", mock.AnythingOfType("*domain.ReportFilters")).Return(nilRows, 0, errors.New("db error"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/reports/party-ledger?gstin=29AABCT1332L1ZP", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.PartyLedger(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestReportHandler_InvalidToDate(t *testing.T) {
	h, _ := newReportHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/reports/buyers?to=13-2025-01", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.Buyers(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.False(t, resp.Success)
	assert.Equal(t, "INVALID_REQUEST", resp.Error.Code)
}

func TestReportHandler_InvalidOffset(t *testing.T) {
	h, _ := newReportHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/reports/sellers?offset=abc", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.Sellers(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.False(t, resp.Success)
	assert.Equal(t, "INVALID_REQUEST", resp.Error.Code)
}

func TestReportHandler_InvalidLimit(t *testing.T) {
	h, _ := newReportHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/reports/sellers?limit=xyz", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.Sellers(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.False(t, resp.Success)
	assert.Equal(t, "INVALID_REQUEST", resp.Error.Code)
}
