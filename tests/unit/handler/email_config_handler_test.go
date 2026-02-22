package handler_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
	"satvos/internal/handler"
	"satvos/internal/service"
	"satvos/mocks"
)

func TestEmailConfigHandler_GetConfig_Success(t *testing.T) {
	mockSvc := new(mocks.MockEmailConfigService)
	h := handler.NewEmailConfigHandler(mockSvc)

	tenantID := uuid.New()
	userID := uuid.New()

	expected := &service.EmailConfigOutput{
		TenantSlug:        "acme",
		Enabled:           true,
		AllowedSenders:    []string{"@acme.com"},
		APIBaseURL:        "https://api.satvos.com",
		HasServiceAccount: true,
	}
	mockSvc.On("GetConfig", mock.Anything, tenantID).Return(expected, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/email-config", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.GetConfig(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	mockSvc.AssertExpectations(t)
}

func TestEmailConfigHandler_GetConfig_ServiceDisabled(t *testing.T) {
	h := handler.NewEmailConfigHandler(nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/email-config", http.NoBody)
	tenantID := uuid.New()
	userID := uuid.New()
	setAuthContext(c, tenantID, userID, "admin")

	h.GetConfig(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestEmailConfigHandler_GetConfig_ServiceError(t *testing.T) {
	mockSvc := new(mocks.MockEmailConfigService)
	h := handler.NewEmailConfigHandler(mockSvc)

	tenantID := uuid.New()
	userID := uuid.New()

	mockSvc.On("GetConfig", mock.Anything, tenantID).Return(nil, errors.New("db error"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/email-config", http.NoBody)
	setAuthContext(c, tenantID, userID, "admin")

	h.GetConfig(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestEmailConfigHandler_UpdateConfig_Success(t *testing.T) {
	mockSvc := new(mocks.MockEmailConfigService)
	h := handler.NewEmailConfigHandler(mockSvc)

	tenantID := uuid.New()
	userID := uuid.New()

	expected := &service.EmailConfigOutput{
		TenantSlug:        "acme",
		Enabled:           true,
		AllowedSenders:    []string{"@acme.com"},
		APIBaseURL:        "https://api.satvos.com",
		HasServiceAccount: true,
	}
	mockSvc.On("UpdateConfig", mock.Anything, tenantID, userID, mock.AnythingOfType("service.UpdateEmailConfigInput")).Return(expected, nil)

	body, _ := json.Marshal(map[string]interface{}{
		"enabled":         true,
		"allowed_senders": []string{"@acme.com"},
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/admin/email-config", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	setAuthContext(c, tenantID, userID, "admin")

	h.UpdateConfig(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	mockSvc.AssertExpectations(t)
}

func TestEmailConfigHandler_UpdateConfig_NoFieldsProvided(t *testing.T) {
	mockSvc := new(mocks.MockEmailConfigService)
	h := handler.NewEmailConfigHandler(mockSvc)

	tenantID := uuid.New()
	userID := uuid.New()

	body, _ := json.Marshal(map[string]interface{}{})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/admin/email-config", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	setAuthContext(c, tenantID, userID, "admin")

	h.UpdateConfig(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.False(t, resp.Success)
	assert.Equal(t, "NO_FIELDS", resp.Error.Code)
}

func TestEmailConfigHandler_UpdateConfig_InvalidSender(t *testing.T) {
	mockSvc := new(mocks.MockEmailConfigService)
	h := handler.NewEmailConfigHandler(mockSvc)

	tenantID := uuid.New()
	userID := uuid.New()

	mockSvc.On("UpdateConfig", mock.Anything, tenantID, userID, mock.AnythingOfType("service.UpdateEmailConfigInput")).
		Return(nil, fmt.Errorf("%w: %q is not a valid email address", domain.ErrInvalidAllowedSender, "bad-email"))

	body, _ := json.Marshal(map[string]interface{}{
		"allowed_senders": []string{"bad-email"},
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/admin/email-config", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	setAuthContext(c, tenantID, userID, "admin")

	h.UpdateConfig(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestEmailConfigHandler_UpdateConfig_ServiceDisabled(t *testing.T) {
	h := handler.NewEmailConfigHandler(nil)

	body, _ := json.Marshal(map[string]interface{}{
		"enabled": true,
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/admin/email-config", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	tenantID := uuid.New()
	userID := uuid.New()
	setAuthContext(c, tenantID, userID, "admin")

	h.UpdateConfig(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
