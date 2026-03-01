package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
	"satvos/internal/handler"
	"satvos/mocks"
)

// --- ListConnectors ---

func TestConnectorHandler_ListConnectors_Success(t *testing.T) {
	syncRepo := new(mocks.MockSyncRepo)
	h := handler.NewConnectorHandler(syncRepo, nil, nil, nil, "")

	tenantID := uuid.New()
	userID := uuid.New()

	agents := []domain.ConnectorAgent{
		{ID: uuid.New(), TenantID: tenantID, AgentVersion: "1.0.0", Status: domain.AgentStatusOnline},
	}
	syncRepo.On("ListAgents", mock.Anything, tenantID).Return(agents, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/connectors", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.ListConnectors(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	syncRepo.AssertExpectations(t)
}

func TestConnectorHandler_ListConnectors_NilRepo(t *testing.T) {
	h := handler.NewConnectorHandler(nil, nil, nil, nil, "")

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/connectors", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.ListConnectors(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// --- ListLedgers ---

func TestConnectorHandler_ListLedgers_Success(t *testing.T) {
	masterRepo := new(mocks.MockTallyMasterRepo)
	h := handler.NewConnectorHandler(nil, masterRepo, nil, nil, "")

	tenantID := uuid.New()
	userID := uuid.New()

	ledgers := []domain.TallyLedger{
		{ID: uuid.New(), TenantID: tenantID, Name: "CGST@9%"},
	}
	masterRepo.On("ListLedgersPaginated", mock.Anything, tenantID, "", "", "", 0, 20).Return(ledgers, 1, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/tally-masters/ledgers", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.ListLedgers(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	assert.NotNil(t, resp.Meta)
	assert.Equal(t, 1, resp.Meta.Total)
	masterRepo.AssertExpectations(t)
}

func TestConnectorHandler_ListLedgers_WithFilters(t *testing.T) {
	masterRepo := new(mocks.MockTallyMasterRepo)
	h := handler.NewConnectorHandler(nil, masterRepo, nil, nil, "")

	tenantID := uuid.New()
	userID := uuid.New()

	ledgers := []domain.TallyLedger{
		{ID: uuid.New(), TenantID: tenantID, Name: "CGST@9%", TaxType: "cgst"},
	}
	masterRepo.On("ListLedgersPaginated", mock.Anything, tenantID, "Duties & Taxes", "cgst", "CGST", 0, 5).Return(ledgers, 1, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/tally-masters/ledgers?parent_group=Duties+%26+Taxes&tax_type=cgst&search=CGST&limit=5", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.ListLedgers(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	masterRepo.AssertExpectations(t)
}

// --- ListStockItems ---

func TestConnectorHandler_ListStockItems_Success(t *testing.T) {
	masterRepo := new(mocks.MockTallyMasterRepo)
	h := handler.NewConnectorHandler(nil, masterRepo, nil, nil, "")

	tenantID := uuid.New()
	userID := uuid.New()

	items := []domain.TallyStockItem{
		{ID: uuid.New(), TenantID: tenantID, Name: "Laptop"},
	}
	masterRepo.On("ListStockItemsPaginated", mock.Anything, tenantID, "", "", "", 0, 20).Return(items, 1, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/tally-masters/stock-items", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.ListStockItems(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	assert.NotNil(t, resp.Meta)
	masterRepo.AssertExpectations(t)
}

func TestConnectorHandler_ListStockItems_WithFilters(t *testing.T) {
	masterRepo := new(mocks.MockTallyMasterRepo)
	h := handler.NewConnectorHandler(nil, masterRepo, nil, nil, "")

	tenantID := uuid.New()
	userID := uuid.New()

	items := []domain.TallyStockItem{
		{ID: uuid.New(), TenantID: tenantID, Name: "Laptop", HSNCode: "8471"},
	}
	masterRepo.On("ListStockItemsPaginated", mock.Anything, tenantID, "Electronics", "8471", "Lap", 0, 20).Return(items, 1, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/tally-masters/stock-items?parent_group=Electronics&hsn_code=8471&search=Lap", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.ListStockItems(c)

	assert.Equal(t, http.StatusOK, w.Code)
	masterRepo.AssertExpectations(t)
}

// --- ListGodowns ---

func TestConnectorHandler_ListGodowns_Success(t *testing.T) {
	masterRepo := new(mocks.MockTallyMasterRepo)
	h := handler.NewConnectorHandler(nil, masterRepo, nil, nil, "")

	tenantID := uuid.New()
	userID := uuid.New()

	godowns := []domain.TallyGodown{
		{ID: uuid.New(), TenantID: tenantID, Name: "Main Location"},
	}
	masterRepo.On("ListGodowns", mock.Anything, tenantID).Return(godowns, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/tally-masters/godowns", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.ListGodowns(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Nil(t, resp.Meta) // unpaginated
	masterRepo.AssertExpectations(t)
}

// --- ListUnits ---

func TestConnectorHandler_ListUnits_Success(t *testing.T) {
	masterRepo := new(mocks.MockTallyMasterRepo)
	h := handler.NewConnectorHandler(nil, masterRepo, nil, nil, "")

	tenantID := uuid.New()
	userID := uuid.New()

	units := []domain.TallyUnit{
		{ID: uuid.New(), TenantID: tenantID, Symbol: "Nos"},
	}
	masterRepo.On("ListUnits", mock.Anything, tenantID).Return(units, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/tally-masters/units", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.ListUnits(c)

	assert.Equal(t, http.StatusOK, w.Code)
	masterRepo.AssertExpectations(t)
}

// --- ListCostCentres ---

//nolint:misspell // Tally uses British spelling for "cost centres"
func TestConnectorHandler_ListCostCentres_Success(t *testing.T) {
	masterRepo := new(mocks.MockTallyMasterRepo)
	h := handler.NewConnectorHandler(nil, masterRepo, nil, nil, "")

	tenantID := uuid.New()
	userID := uuid.New()

	centers := []domain.TallyCostCentre{ //nolint:misspell // domain type uses British spelling
		{ID: uuid.New(), TenantID: tenantID, Name: "Head Office"},
	}
	masterRepo.On("ListCostCentres", mock.Anything, tenantID).Return(centers, nil) //nolint:misspell // interface method

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/tally-masters/cost-centres", http.NoBody) //nolint:misspell // API path
	setAuthContext(c, tenantID, userID, "admin")

	h.ListCostCentres(c) //nolint:misspell // method name

	assert.Equal(t, http.StatusOK, w.Code)
	masterRepo.AssertExpectations(t)
}

// --- ListTallyVouchers ---

func TestConnectorHandler_ListTallyVouchers_Success(t *testing.T) {
	voucherRepo := new(mocks.MockTallyVoucherRepo)
	h := handler.NewConnectorHandler(nil, nil, voucherRepo, nil, "")

	tenantID := uuid.New()
	userID := uuid.New()

	vouchers := []domain.TallyVoucher{
		{ID: uuid.New(), TenantID: tenantID, VoucherType: "Purchase", Amount: 1000},
	}
	voucherRepo.On("ListByTenantFiltered", mock.Anything, tenantID, "", "", (*time.Time)(nil), (*time.Time)(nil), 0, 20).Return(vouchers, 1, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/tally-vouchers", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.ListTallyVouchers(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	assert.NotNil(t, resp.Meta)
	voucherRepo.AssertExpectations(t)
}

func TestConnectorHandler_ListTallyVouchers_WithDateFilter(t *testing.T) {
	voucherRepo := new(mocks.MockTallyVoucherRepo)
	h := handler.NewConnectorHandler(nil, nil, voucherRepo, nil, "")

	tenantID := uuid.New()
	userID := uuid.New()

	fromDate, _ := time.Parse("2006-01-02", "2025-01-01")
	toDate, _ := time.Parse("2006-01-02", "2025-12-31")

	vouchers := []domain.TallyVoucher{
		{ID: uuid.New(), TenantID: tenantID, VoucherType: "Purchase"},
	}
	voucherRepo.On("ListByTenantFiltered", mock.Anything, tenantID, "Purchase", "", &fromDate, &toDate, 0, 20).Return(vouchers, 1, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/tally-vouchers?voucher_type=Purchase&from=2025-01-01&to=2025-12-31", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.ListTallyVouchers(c)

	assert.Equal(t, http.StatusOK, w.Code)
	voucherRepo.AssertExpectations(t)
}

func TestConnectorHandler_ListTallyVouchers_InvalidDate(t *testing.T) {
	voucherRepo := new(mocks.MockTallyVoucherRepo)
	h := handler.NewConnectorHandler(nil, nil, voucherRepo, nil, "")

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/tally-vouchers?from=not-a-date", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.ListTallyVouchers(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- GetTallyVoucher ---

func TestConnectorHandler_GetTallyVoucher_Success(t *testing.T) {
	voucherRepo := new(mocks.MockTallyVoucherRepo)
	h := handler.NewConnectorHandler(nil, nil, voucherRepo, nil, "")

	tenantID := uuid.New()
	userID := uuid.New()
	voucherID := uuid.New()

	voucher := &domain.TallyVoucher{
		ID:          voucherID,
		TenantID:    tenantID,
		VoucherType: "Purchase",
		Amount:      5000,
	}
	voucherRepo.On("GetByID", mock.Anything, tenantID, voucherID).Return(voucher, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/tally-vouchers/"+voucherID.String(), http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: voucherID.String()}}
	setAuthContext(c, tenantID, userID, "admin")

	h.GetTallyVoucher(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	voucherRepo.AssertExpectations(t)
}

func TestConnectorHandler_GetTallyVoucher_NotFound(t *testing.T) {
	voucherRepo := new(mocks.MockTallyVoucherRepo)
	h := handler.NewConnectorHandler(nil, nil, voucherRepo, nil, "")

	tenantID := uuid.New()
	userID := uuid.New()
	voucherID := uuid.New()

	voucherRepo.On("GetByID", mock.Anything, tenantID, voucherID).Return(nil, domain.ErrNotFound)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/tally-vouchers/"+voucherID.String(), http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: voucherID.String()}}
	setAuthContext(c, tenantID, userID, "admin")

	h.GetTallyVoucher(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestConnectorHandler_GetTallyVoucher_InvalidID(t *testing.T) {
	voucherRepo := new(mocks.MockTallyVoucherRepo)
	h := handler.NewConnectorHandler(nil, nil, voucherRepo, nil, "")

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/tally-vouchers/bad-id", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: "bad-id"}}
	setAuthContext(c, tenantID, userID, "admin")

	h.GetTallyVoucher(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- DownloadConnector ---

func TestConnectorHandler_DownloadConnector_Success(t *testing.T) {
	storage := new(mocks.MockObjectStorage)
	h := handler.NewConnectorHandler(nil, nil, nil, storage, "satvos-uploads")

	tenantID := uuid.New()
	userID := uuid.New()

	storage.On("GetPresignedURL", mock.Anything, "satvos-uploads", "connectors/tally/latest/satvos-connector.exe", int64(300)).
		Return("https://satvos-uploads.s3.amazonaws.com/connectors/tally/latest/satvos-connector.exe?signed", nil)
	storage.On("GetPresignedURL", mock.Anything, "satvos-uploads", "connectors/tally/latest/satvos-connector.exe.sha256", int64(300)).
		Return("https://satvos-uploads.s3.amazonaws.com/connectors/tally/latest/satvos-connector.exe.sha256?signed", nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/connector-download", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.DownloadConnector(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)

	data, ok := resp.Data.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "https://satvos-uploads.s3.amazonaws.com/connectors/tally/latest/satvos-connector.exe?signed", data["download_url"])
	assert.Equal(t, "https://satvos-uploads.s3.amazonaws.com/connectors/tally/latest/satvos-connector.exe.sha256?signed", data["checksum_url"])
	assert.Equal(t, "satvos-connector.exe", data["filename"])
	storage.AssertExpectations(t)
}

func TestConnectorHandler_DownloadConnector_NilStorage(t *testing.T) {
	h := handler.NewConnectorHandler(nil, nil, nil, nil, "")

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/connector-download", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.DownloadConnector(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestConnectorHandler_DownloadConnector_StorageError(t *testing.T) {
	storage := new(mocks.MockObjectStorage)
	h := handler.NewConnectorHandler(nil, nil, nil, storage, "satvos-uploads")

	tenantID := uuid.New()
	userID := uuid.New()

	storage.On("GetPresignedURL", mock.Anything, "satvos-uploads", "connectors/tally/latest/satvos-connector.exe", int64(300)).
		Return("", assert.AnError)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/connector-download", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.DownloadConnector(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	storage.AssertExpectations(t)
}
