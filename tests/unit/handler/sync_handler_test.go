package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"satvos/internal/domain"
	"satvos/internal/handler"
	"satvos/internal/service"
	"satvos/mocks"
)

func newSyncHandler() (*handler.SyncHandler, *mocks.MockSyncService) {
	mockSvc := new(mocks.MockSyncService)
	h := handler.NewSyncHandler(mockSvc)
	return h, mockSvc
}

// --- Register ---

func TestSyncHandler_Register_Success(t *testing.T) {
	h, mockSvc := newSyncHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	expected := &domain.ConnectorAgent{
		ID:               uuid.New(),
		TenantID:         tenantID,
		ServiceAccountID: userID,
		AgentVersion:     "1.0.0",
		OSInfo:           "Windows 10",
		Status:           domain.AgentStatusRegistered,
	}

	mockSvc.On("Register", mock.Anything, tenantID, userID, "1.0.0", "Windows 10").Return(expected, nil)

	body, _ := json.Marshal(map[string]string{
		"version": "1.0.0",
		"os_info": "Windows 10",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/sync/v1/register", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	setAuthContext(c, tenantID, userID, "service")

	h.Register(c)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.NotNil(t, resp.Data)
	mockSvc.AssertExpectations(t)
}

func TestSyncHandler_Register_MissingAuth(t *testing.T) {
	h, _ := newSyncHandler()

	body, _ := json.Marshal(map[string]string{
		"version": "1.0.0",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/sync/v1/register", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.Register(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSyncHandler_Register_InvalidBody(t *testing.T) {
	h, _ := newSyncHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	body, _ := json.Marshal(map[string]string{
		"os_info": "Linux",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/sync/v1/register", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	setAuthContext(c, tenantID, userID, "service")

	h.Register(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.False(t, resp.Success)
}

// --- Heartbeat ---

func TestSyncHandler_Heartbeat_Success(t *testing.T) {
	h, mockSvc := newSyncHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	mockSvc.On("Heartbeat", mock.Anything, tenantID, userID, true, "My Company", 9000, "1.0.0", []string(nil)).Return(nil)

	body, _ := json.Marshal(map[string]interface{}{
		"tally_connected": true,
		"tally_company":   "My Company",
		"tally_port":      9000,
		"version":         "1.0.0",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/sync/v1/heartbeat", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	setAuthContext(c, tenantID, userID, "service")

	h.Heartbeat(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.True(t, resp.Success)
	mockSvc.AssertExpectations(t)
}

// --- Masters ---

func TestSyncHandler_Masters_Success(t *testing.T) {
	h, mockSvc := newSyncHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	mockSvc.On("SaveMasters", mock.Anything, tenantID, mock.MatchedBy(func(p *service.MasterPayload) bool {
		return len(p.Ledgers) == 1 && p.Ledgers[0].Name == "Purchase Account" &&
			len(p.StockItems) == 1 && p.StockItems[0].Name == "Widget"
	})).Return(nil)

	payload := service.MasterPayload{
		Ledgers: []domain.TallyLedger{
			{Name: "Purchase Account", ParentGroup: "Purchase Accounts"},
		},
		StockItems: []domain.TallyStockItem{
			{Name: "Widget", HSNCode: "8471"},
		},
	}
	body, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/sync/v1/masters", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	setAuthContext(c, tenantID, userID, "service")

	h.Masters(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.True(t, resp.Success)
	mockSvc.AssertExpectations(t)
}

// --- Outbound ---

func TestSyncHandler_Outbound_Success(t *testing.T) {
	h, mockSvc := newSyncHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	items := []service.OutboundItem{
		{
			DocumentID:  uuid.New(),
			SyncEventID: uuid.New(),
			VoucherDef:  &domain.VoucherDef{VoucherType: "Purchase"},
		},
	}

	mockSvc.On("ListOutbound", mock.Anything, tenantID, "", 50).Return(items, "next-abc", nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/sync/v1/outbound", http.NoBody)
	setAuthContext(c, tenantID, userID, "service")

	h.Outbound(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.True(t, resp.Success)

	dataMap, ok := resp.Data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "next-abc", dataMap["next_cursor"])
	assert.NotNil(t, dataMap["items"])
	mockSvc.AssertExpectations(t)
}

// --- Ack ---

func TestSyncHandler_Ack_Success(t *testing.T) {
	h, mockSvc := newSyncHandler()

	tenantID := uuid.New()
	userID := uuid.New()
	syncEventID := uuid.New()
	docID := uuid.New()

	results := []service.AckResult{
		{
			SyncEventID:        syncEventID,
			DocumentID:         docID,
			Success:            true,
			TallyVoucherID:     "TV-001",
			TallyVoucherNumber: "PUR/2024-25/001",
		},
	}

	mockSvc.On("AckOutbound", mock.Anything, tenantID, userID, mock.MatchedBy(func(r []service.AckResult) bool {
		return len(r) == 1 && r[0].SyncEventID == syncEventID && r[0].Success
	})).Return(nil)

	body, _ := json.Marshal(map[string]interface{}{
		"results": results,
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/sync/v1/ack", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	setAuthContext(c, tenantID, userID, "service")

	h.Ack(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.True(t, resp.Success)
	mockSvc.AssertExpectations(t)
}

// --- Inbound ---

func TestSyncHandler_Inbound_Success(t *testing.T) {
	h, mockSvc := newSyncHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	vouchers := []domain.TallyVoucher{
		{
			VoucherType:   "Sales",
			VoucherNumber: "SAL/001",
			PartyName:     "Acme Corp",
			Amount:        10000.00,
		},
	}

	mockSvc.On("SaveInbound", mock.Anything, tenantID, mock.MatchedBy(func(v []domain.TallyVoucher) bool {
		return len(v) == 1 && v[0].VoucherType == "Sales" && v[0].PartyName == "Acme Corp"
	})).Return(nil)

	body, _ := json.Marshal(map[string]interface{}{
		"vouchers": vouchers,
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/sync/v1/inbound", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	setAuthContext(c, tenantID, userID, "service")

	h.Inbound(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.True(t, resp.Success)
	mockSvc.AssertExpectations(t)
}
