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
	"satvos/internal/service"
	"satvos/mocks"
)

func newUserHandler() (*handler.UserHandler, *mocks.MockUserService) {
	mockSvc := new(mocks.MockUserService)
	h := handler.NewUserHandler(mockSvc, nil)
	return h, mockSvc
}

// --- Create ---

func TestUserHandler_Create_Success(t *testing.T) {
	h, mockSvc := newUserHandler()

	tenantID := uuid.New()
	adminID := uuid.New()
	newUserID := uuid.New()

	expected := &domain.User{
		ID:       newUserID,
		TenantID: tenantID,
		Email:    "jane@acme.com",
		FullName: "Jane Doe",
		Role:     domain.RoleMember,
		IsActive: true,
	}

	mockSvc.On("Create", mock.Anything, tenantID, mock.MatchedBy(func(input service.CreateUserInput) bool {
		return input.Email == "jane@acme.com" && input.FullName == "Jane Doe"
	})).Return(expected, nil)

	body, _ := json.Marshal(map[string]string{
		"email":     "jane@acme.com",
		"password":  "securepassword",
		"full_name": "Jane Doe",
		"role":      "member",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	setAuthContext(c, tenantID, adminID, "admin")

	h.Create(c)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	mockSvc.AssertExpectations(t)
}

func TestUserHandler_Create_NoAuth(t *testing.T) {
	h, _ := newUserHandler()

	body, _ := json.Marshal(map[string]string{
		"email":     "jane@acme.com",
		"password":  "password",
		"full_name": "Jane",
		"role":      "member",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.Create(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestUserHandler_Create_DuplicateEmail(t *testing.T) {
	h, mockSvc := newUserHandler()

	tenantID := uuid.New()
	adminID := uuid.New()

	mockSvc.On("Create", mock.Anything, tenantID, mock.AnythingOfType("service.CreateUserInput")).
		Return(nil, domain.ErrDuplicateEmail)

	body, _ := json.Marshal(map[string]string{
		"email":     "existing@acme.com",
		"password":  "password123",
		"full_name": "Existing User",
		"role":      "member",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	setAuthContext(c, tenantID, adminID, "admin")

	h.Create(c)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestUserHandler_Create_MissingFields(t *testing.T) {
	h, _ := newUserHandler()

	tenantID := uuid.New()
	adminID := uuid.New()

	// Missing required fields
	body, _ := json.Marshal(map[string]string{
		"email": "jane@acme.com",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	setAuthContext(c, tenantID, adminID, "admin")

	h.Create(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- List ---

func TestUserHandler_List_Success(t *testing.T) {
	h, mockSvc := newUserHandler()

	tenantID := uuid.New()
	adminID := uuid.New()

	users := []domain.User{
		{ID: uuid.New(), TenantID: tenantID, Email: "user1@acme.com", IsActive: true},
		{ID: uuid.New(), TenantID: tenantID, Email: "user2@acme.com", IsActive: true},
	}

	mockSvc.On("List", mock.Anything, tenantID, 0, 20).Return(users, 2, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/users?offset=0&limit=20", http.NoBody)
	setAuthContext(c, tenantID, adminID, "admin")

	h.List(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	assert.NotNil(t, resp.Meta)
	assert.Equal(t, 2, resp.Meta.Total)
	mockSvc.AssertExpectations(t)
}

func TestUserHandler_List_NoAuth(t *testing.T) {
	h, _ := newUserHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/users", http.NoBody)

	h.List(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// --- GetByID ---

func TestUserHandler_GetByID_SelfAccess(t *testing.T) {
	h, mockSvc := newUserHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	expected := &domain.User{
		ID:       userID,
		TenantID: tenantID,
		Email:    "user@acme.com",
		Role:     domain.RoleMember,
	}

	mockSvc.On("GetByID", mock.Anything, tenantID, userID).Return(expected, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/users/"+userID.String(), http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: userID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.GetByID(c)

	assert.Equal(t, http.StatusOK, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestUserHandler_GetByID_AdminAccess(t *testing.T) {
	h, mockSvc := newUserHandler()

	tenantID := uuid.New()
	adminID := uuid.New()
	targetUserID := uuid.New()

	expected := &domain.User{
		ID:       targetUserID,
		TenantID: tenantID,
		Email:    "target@acme.com",
	}

	mockSvc.On("GetByID", mock.Anything, tenantID, targetUserID).Return(expected, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/users/"+targetUserID.String(), http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: targetUserID.String()}}
	setAuthContext(c, tenantID, adminID, "admin")

	h.GetByID(c)

	assert.Equal(t, http.StatusOK, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestUserHandler_GetByID_MemberAccessOtherUser(t *testing.T) {
	h, _ := newUserHandler()

	tenantID := uuid.New()
	memberID := uuid.New()
	otherUserID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/users/"+otherUserID.String(), http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: otherUserID.String()}}
	setAuthContext(c, tenantID, memberID, "member")

	h.GetByID(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestUserHandler_GetByID_InvalidID(t *testing.T) {
	h, _ := newUserHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/users/bad-id", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: "bad-id"}}
	setAuthContext(c, tenantID, userID, "admin")

	h.GetByID(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUserHandler_GetByID_NotFound(t *testing.T) {
	h, mockSvc := newUserHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	mockSvc.On("GetByID", mock.Anything, tenantID, userID).Return(nil, domain.ErrNotFound)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/users/"+userID.String(), http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: userID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.GetByID(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// --- Update ---

func TestUserHandler_Update_SelfUpdate(t *testing.T) {
	h, mockSvc := newUserHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	updated := &domain.User{
		ID:       userID,
		TenantID: tenantID,
		Email:    "newemail@acme.com",
		FullName: "New Name",
		Role:     domain.RoleMember,
	}

	mockSvc.On("Update", mock.Anything, tenantID, userID, mock.AnythingOfType("service.UpdateUserInput")).
		Return(updated, nil)

	body, _ := json.Marshal(map[string]string{
		"full_name": "New Name",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/users/"+userID.String(), bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: userID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.Update(c)

	assert.Equal(t, http.StatusOK, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestUserHandler_Update_MemberCannotChangeRole(t *testing.T) {
	h, _ := newUserHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	role := domain.RoleAdmin
	body, _ := json.Marshal(service.UpdateUserInput{
		Role: &role,
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/users/"+userID.String(), bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: userID.String()}}
	setAuthContext(c, tenantID, userID, "member")

	h.Update(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestUserHandler_Update_AdminCanChangeRole(t *testing.T) {
	h, mockSvc := newUserHandler()

	tenantID := uuid.New()
	adminID := uuid.New()
	targetUserID := uuid.New()

	updated := &domain.User{
		ID:   targetUserID,
		Role: domain.RoleAdmin,
	}

	mockSvc.On("Update", mock.Anything, tenantID, targetUserID, mock.AnythingOfType("service.UpdateUserInput")).
		Return(updated, nil)

	role := domain.RoleAdmin
	body, _ := json.Marshal(service.UpdateUserInput{
		Role: &role,
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/users/"+targetUserID.String(), bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: targetUserID.String()}}
	setAuthContext(c, tenantID, adminID, "admin")

	h.Update(c)

	assert.Equal(t, http.StatusOK, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestUserHandler_Update_MemberCannotUpdateOther(t *testing.T) {
	h, _ := newUserHandler()

	tenantID := uuid.New()
	memberID := uuid.New()
	otherUserID := uuid.New()

	body, _ := json.Marshal(map[string]string{"full_name": "Hacked"})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/users/"+otherUserID.String(), bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: otherUserID.String()}}
	setAuthContext(c, tenantID, memberID, "member")

	h.Update(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestUserHandler_Update_InvalidID(t *testing.T) {
	h, _ := newUserHandler()

	tenantID := uuid.New()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPut, "/api/v1/users/bad-id", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: "bad-id"}}
	setAuthContext(c, tenantID, userID, "admin")

	h.Update(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- Delete ---

func TestUserHandler_Delete_Success(t *testing.T) {
	h, mockSvc := newUserHandler()

	tenantID := uuid.New()
	adminID := uuid.New()
	targetUserID := uuid.New()

	mockSvc.On("Delete", mock.Anything, tenantID, targetUserID).Return(nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodDelete, "/api/v1/users/"+targetUserID.String(), http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: targetUserID.String()}}
	setAuthContext(c, tenantID, adminID, "admin")

	h.Delete(c)

	assert.Equal(t, http.StatusOK, w.Code)
	mockSvc.AssertExpectations(t)
}

func TestUserHandler_Delete_NotFound(t *testing.T) {
	h, mockSvc := newUserHandler()

	tenantID := uuid.New()
	adminID := uuid.New()
	targetUserID := uuid.New()

	mockSvc.On("Delete", mock.Anything, tenantID, targetUserID).Return(domain.ErrNotFound)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodDelete, "/api/v1/users/"+targetUserID.String(), http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: targetUserID.String()}}
	setAuthContext(c, tenantID, adminID, "admin")

	h.Delete(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUserHandler_Delete_NoAuth(t *testing.T) {
	h, _ := newUserHandler()

	targetUserID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodDelete, "/api/v1/users/"+targetUserID.String(), http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: targetUserID.String()}}

	h.Delete(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestUserHandler_Delete_InvalidID(t *testing.T) {
	h, _ := newUserHandler()

	tenantID := uuid.New()
	adminID := uuid.New()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodDelete, "/api/v1/users/bad-id", http.NoBody)
	c.Params = gin.Params{{Key: "id", Value: "bad-id"}}
	setAuthContext(c, tenantID, adminID, "admin")

	h.Delete(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
