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

	"satvos/internal/domain"
	"satvos/internal/handler"
	"satvos/internal/middleware"
	"satvos/internal/service"
	"satvos/mocks"
)

func newTenantHandler() (*handler.TenantHandler, *mocks.MockTenantService) {
	mockSvc := new(mocks.MockTenantService)
	h := handler.NewTenantHandler(mockSvc)
	return h, mockSvc
}

func setTenantAuthContext(c *gin.Context, tenantID, userID uuid.UUID) {
	c.Set(middleware.ContextKeyTenantID, tenantID)
	c.Set(middleware.ContextKeyUserID, userID)
	c.Set(middleware.ContextKeyRole, "admin")
	c.Set(middleware.ContextKeyEmail, "admin@test.com")
}

// ==================== GetOwnTenant ====================

func TestTenantHandler_GetOwnTenant_Success(t *testing.T) {
	h, mockSvc := newTenantHandler()

	tenantID := uuid.New()
	adminID := uuid.New()
	expected := &domain.Tenant{
		ID:       tenantID,
		Name:     "Acme Corp",
		Slug:     "acme",
		IsActive: true,
	}

	mockSvc.On("GetByID", mock.Anything, tenantID).Return(expected, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/tenant", http.NoBody)
	setTenantAuthContext(c, tenantID, adminID)

	h.GetOwnTenant(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	mockSvc.AssertExpectations(t)
}

func TestTenantHandler_GetOwnTenant_NoAuth(t *testing.T) {
	h, _ := newTenantHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/tenant", http.NoBody)
	// no auth context set

	h.GetOwnTenant(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestTenantHandler_GetOwnTenant_NotFound(t *testing.T) {
	h, mockSvc := newTenantHandler()

	tenantID := uuid.New()
	adminID := uuid.New()
	mockSvc.On("GetByID", mock.Anything, tenantID).Return(nil, domain.ErrNotFound)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/tenant", http.NoBody)
	setTenantAuthContext(c, tenantID, adminID)

	h.GetOwnTenant(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ==================== UpdateOwnTenant ====================

func TestTenantHandler_UpdateOwnTenant_Success(t *testing.T) {
	h, mockSvc := newTenantHandler()

	tenantID := uuid.New()
	adminID := uuid.New()
	updated := &domain.Tenant{
		ID:       tenantID,
		Name:     "Acme Industries",
		Slug:     "acme",
		IsActive: true,
	}

	mockSvc.On("Update", mock.Anything, tenantID, mock.AnythingOfType("service.UpdateTenantInput")).
		Return(updated, nil)

	body, _ := json.Marshal(map[string]interface{}{
		"name": "Acme Industries",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/admin/tenant", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	setTenantAuthContext(c, tenantID, adminID)

	h.UpdateOwnTenant(c)

	assert.Equal(t, http.StatusOK, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestTenantHandler_UpdateOwnTenant_NoAuth(t *testing.T) {
	h, _ := newTenantHandler()

	body, _ := json.Marshal(map[string]string{"name": "New"})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/admin/tenant", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateOwnTenant(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestTenantHandler_UpdateOwnTenant_BadRequest(t *testing.T) {
	h, _ := newTenantHandler()

	tenantID := uuid.New()
	adminID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/admin/tenant", bytes.NewReader([]byte("not-json")))
	c.Request.Header.Set("Content-Type", "application/json")
	setTenantAuthContext(c, tenantID, adminID)

	h.UpdateOwnTenant(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTenantHandler_UpdateOwnTenant_StripsIsActive(t *testing.T) {
	h, mockSvc := newTenantHandler()

	tenantID := uuid.New()
	adminID := uuid.New()
	updated := &domain.Tenant{
		ID:       tenantID,
		Name:     "Acme Industries",
		Slug:     "acme",
		IsActive: true,
	}

	// Verify the service receives input with IsActive stripped (nil)
	mockSvc.On("Update", mock.Anything, tenantID, mock.MatchedBy(func(input service.UpdateTenantInput) bool {
		return input.IsActive == nil
	})).Return(updated, nil)

	body, _ := json.Marshal(map[string]interface{}{
		"name":      "Acme Industries",
		"is_active": false, // This should be stripped by the handler
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/admin/tenant", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	setTenantAuthContext(c, tenantID, adminID)

	h.UpdateOwnTenant(c)

	assert.Equal(t, http.StatusOK, w.Code)
	mockSvc.AssertExpectations(t)
}

// ==================== Create (disabled) ====================

func TestTenantHandler_Create_Forbidden(t *testing.T) {
	h, _ := newTenantHandler()

	body, _ := json.Marshal(map[string]string{
		"name": "Acme Corp",
		"slug": "acme",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/admin/tenants", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	setTenantAuthContext(c, uuid.New(), uuid.New())

	h.Create(c)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.False(t, resp.Success)
	assert.Equal(t, "FORBIDDEN", resp.Error.Code)
}

// ==================== Delete (disabled) ====================

func TestTenantHandler_Delete_Forbidden(t *testing.T) {
	h, _ := newTenantHandler()

	tenantID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodDelete, "/api/v1/admin/tenants/"+tenantID.String(), http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: tenantID.String()}}
	setTenantAuthContext(c, tenantID, uuid.New())

	h.Delete(c)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.False(t, resp.Success)
	assert.Equal(t, "FORBIDDEN", resp.Error.Code)
}

// ==================== List (returns own tenant only) ====================

func TestTenantHandler_List_ReturnsOwnTenantOnly(t *testing.T) {
	h, mockSvc := newTenantHandler()

	tenantID := uuid.New()
	adminID := uuid.New()
	ownTenant := &domain.Tenant{
		ID:       tenantID,
		Name:     "Acme",
		Slug:     "acme",
		IsActive: true,
	}

	mockSvc.On("GetByID", mock.Anything, tenantID).Return(ownTenant, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/tenants", http.NoBody)
	setTenantAuthContext(c, tenantID, adminID)

	h.List(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	assert.NotNil(t, resp.Meta)
	assert.Equal(t, 1, resp.Meta.Total)
	mockSvc.AssertExpectations(t)
}

func TestTenantHandler_List_NoAuth(t *testing.T) {
	h, _ := newTenantHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/tenants", http.NoBody)

	h.List(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ==================== GetByID (scoped to own tenant) ====================

func TestTenantHandler_GetByID_OwnTenant_Success(t *testing.T) {
	h, mockSvc := newTenantHandler()

	tenantID := uuid.New()
	adminID := uuid.New()
	expected := &domain.Tenant{
		ID:       tenantID,
		Name:     "Acme",
		Slug:     "acme",
		IsActive: true,
	}

	mockSvc.On("GetByID", mock.Anything, tenantID).Return(expected, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/tenants/"+tenantID.String(), http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: tenantID.String()}}
	setTenantAuthContext(c, tenantID, adminID)

	h.GetByID(c)

	assert.Equal(t, http.StatusOK, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestTenantHandler_GetByID_CrossTenant_Forbidden(t *testing.T) {
	h, _ := newTenantHandler()

	ownTenantID := uuid.New()
	otherTenantID := uuid.New()
	adminID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/tenants/"+otherTenantID.String(), http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: otherTenantID.String()}}
	setTenantAuthContext(c, ownTenantID, adminID)

	h.GetByID(c)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.False(t, resp.Success)
	assert.Equal(t, "FORBIDDEN", resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "own tenant")
}

func TestTenantHandler_GetByID_InvalidID(t *testing.T) {
	h, _ := newTenantHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/tenants/not-a-uuid", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: "not-a-uuid"}}
	setTenantAuthContext(c, uuid.New(), uuid.New())

	h.GetByID(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTenantHandler_GetByID_NoAuth(t *testing.T) {
	h, _ := newTenantHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/tenants/"+uuid.New().String(), http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}

	h.GetByID(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestTenantHandler_GetByID_NotFound(t *testing.T) {
	h, mockSvc := newTenantHandler()

	tenantID := uuid.New()
	mockSvc.On("GetByID", mock.Anything, tenantID).Return(nil, domain.ErrNotFound)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/admin/tenants/"+tenantID.String(), http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: tenantID.String()}}
	setTenantAuthContext(c, tenantID, uuid.New())

	h.GetByID(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ==================== Update (scoped to own tenant) ====================

func TestTenantHandler_Update_OwnTenant_Success(t *testing.T) {
	h, mockSvc := newTenantHandler()

	tenantID := uuid.New()
	adminID := uuid.New()
	updated := &domain.Tenant{
		ID:       tenantID,
		Name:     "Acme Industries",
		Slug:     "acme",
		IsActive: true,
	}

	mockSvc.On("Update", mock.Anything, tenantID, mock.AnythingOfType("service.UpdateTenantInput")).
		Return(updated, nil)

	body, _ := json.Marshal(map[string]interface{}{
		"name":      "Acme Industries",
		"is_active": true,
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/admin/tenants/"+tenantID.String(), bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: tenantID.String()}}
	setTenantAuthContext(c, tenantID, adminID)

	h.Update(c)

	assert.Equal(t, http.StatusOK, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestTenantHandler_Update_CrossTenant_Forbidden(t *testing.T) {
	h, _ := newTenantHandler()

	ownTenantID := uuid.New()
	otherTenantID := uuid.New()
	adminID := uuid.New()

	body, _ := json.Marshal(map[string]string{"name": "Hijacked Name"})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/admin/tenants/"+otherTenantID.String(), bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: otherTenantID.String()}}
	setTenantAuthContext(c, ownTenantID, adminID)

	h.Update(c)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.False(t, resp.Success)
	assert.Equal(t, "FORBIDDEN", resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "own tenant")
}

func TestTenantHandler_Update_InvalidID(t *testing.T) {
	h, _ := newTenantHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/admin/tenants/bad-id", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: "bad-id"}}
	setTenantAuthContext(c, uuid.New(), uuid.New())

	h.Update(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTenantHandler_Update_NoAuth(t *testing.T) {
	h, _ := newTenantHandler()

	body, _ := json.Marshal(map[string]string{"name": "New"})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/admin/tenants/"+uuid.New().String(), bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: uuid.New().String()}}

	h.Update(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestTenantHandler_Update_NotFound(t *testing.T) {
	h, mockSvc := newTenantHandler()

	tenantID := uuid.New()
	mockSvc.On("Update", mock.Anything, tenantID, mock.AnythingOfType("service.UpdateTenantInput")).
		Return(nil, domain.ErrNotFound)

	body, _ := json.Marshal(map[string]string{"name": "New Name"})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/admin/tenants/"+tenantID.String(), bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: tenantID.String()}}
	setTenantAuthContext(c, tenantID, uuid.New())

	h.Update(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
