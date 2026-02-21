package handler_test

import (
	"bytes"
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
	"satvos/internal/service"
	"satvos/mocks"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestAuthHandler_Login_Success(t *testing.T) {
	mockAuth := new(mocks.MockAuthService)
	h := handler.NewAuthHandler(mockAuth, nil, nil, nil, nil)

	tokenPair := &service.TokenPair{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(15 * time.Minute),
	}

	mockAuth.On("Login", mock.Anything, service.LoginInput{
		TenantSlug: "test-tenant",
		Email:      "user@test.com",
		Password:   "password123",
	}).Return(tokenPair, nil)

	body, _ := json.Marshal(map[string]string{
		"tenant_slug": "test-tenant",
		"email":       "user@test.com",
		"password":    "password123",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.Login(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	mockAuth.AssertExpectations(t)
}

func TestAuthHandler_Login_InvalidCredentials(t *testing.T) {
	mockAuth := new(mocks.MockAuthService)
	h := handler.NewAuthHandler(mockAuth, nil, nil, nil, nil)

	mockAuth.On("Login", mock.Anything, mock.AnythingOfType("service.LoginInput")).
		Return(nil, domain.ErrInvalidCredentials)

	body, _ := json.Marshal(map[string]string{
		"tenant_slug": "test-tenant",
		"email":       "user@test.com",
		"password":    "wrongpassword",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.Login(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthHandler_Login_ValidationError(t *testing.T) {
	mockAuth := new(mocks.MockAuthService)
	h := handler.NewAuthHandler(mockAuth, nil, nil, nil, nil)

	// Missing required fields
	body, _ := json.Marshal(map[string]string{
		"email": "not-an-email",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.Login(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuthHandler_RefreshToken_Success(t *testing.T) {
	mockAuth := new(mocks.MockAuthService)
	h := handler.NewAuthHandler(mockAuth, nil, nil, nil, nil)

	tokenPair := &service.TokenPair{
		AccessToken:  "new-access-token",
		RefreshToken: "new-refresh-token",
		ExpiresAt:    time.Now().Add(15 * time.Minute),
	}

	mockAuth.On("RefreshToken", mock.Anything, "valid-refresh-token").Return(tokenPair, nil)

	body, _ := json.Marshal(map[string]string{
		"refresh_token": "valid-refresh-token",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.RefreshToken(c)

	assert.Equal(t, http.StatusOK, w.Code)
	mockAuth.AssertExpectations(t)
}

func TestAuthHandler_VerifyEmail_Success(t *testing.T) {
	mockAuth := new(mocks.MockAuthService)
	mockReg := new(mocks.MockRegistrationService)
	h := handler.NewAuthHandler(mockAuth, mockReg, nil, nil, nil)

	mockReg.On("VerifyEmail", mock.Anything, "valid-token-string").Return(nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/auth/verify-email?token=valid-token-string", http.NoBody)

	h.VerifyEmail(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.True(t, resp.Success)
	mockReg.AssertExpectations(t)
}

func TestAuthHandler_VerifyEmail_MissingToken(t *testing.T) {
	mockAuth := new(mocks.MockAuthService)
	mockReg := new(mocks.MockRegistrationService)
	h := handler.NewAuthHandler(mockAuth, mockReg, nil, nil, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/auth/verify-email", http.NoBody)

	h.VerifyEmail(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuthHandler_VerifyEmail_InvalidToken(t *testing.T) {
	mockAuth := new(mocks.MockAuthService)
	mockReg := new(mocks.MockRegistrationService)
	h := handler.NewAuthHandler(mockAuth, mockReg, nil, nil, nil)

	mockReg.On("VerifyEmail", mock.Anything, "invalid-token").Return(domain.ErrUnauthorized)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/auth/verify-email?token=invalid-token", http.NoBody)

	h.VerifyEmail(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	mockReg.AssertExpectations(t)
}

func TestAuthHandler_VerifyEmail_RegistrationDisabled(t *testing.T) {
	mockAuth := new(mocks.MockAuthService)
	h := handler.NewAuthHandler(mockAuth, nil, nil, nil, nil) // nil registration service

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/auth/verify-email?token=some-token", http.NoBody)

	h.VerifyEmail(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAuthHandler_ResendVerification_Success(t *testing.T) {
	mockAuth := new(mocks.MockAuthService)
	mockReg := new(mocks.MockRegistrationService)
	h := handler.NewAuthHandler(mockAuth, mockReg, nil, nil, nil)

	tenantID := uuid.New()
	userID := uuid.New()

	mockReg.On("ResendVerification", mock.Anything, tenantID, userID).Return(nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/resend-verification", http.NoBody)
	c.Set("tenant_id", tenantID)
	c.Set("user_id", userID)
	c.Set("role", string(domain.RoleFree))

	h.ResendVerification(c)

	assert.Equal(t, http.StatusOK, w.Code)
	mockReg.AssertExpectations(t)
}

func TestAuthHandler_ForgotPassword_Success(t *testing.T) {
	mockAuth := new(mocks.MockAuthService)
	mockPwReset := new(mocks.MockPasswordResetService)
	h := handler.NewAuthHandler(mockAuth, nil, mockPwReset, nil, nil)

	mockPwReset.On("ForgotPassword", mock.Anything, service.ForgotPasswordInput{
		TenantSlug: "test-tenant",
		Email:      "user@test.com",
	}).Return(nil)

	body, _ := json.Marshal(map[string]string{
		"tenant_slug": "test-tenant",
		"email":       "user@test.com",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/forgot-password", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.ForgotPassword(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.True(t, resp.Success)
	mockPwReset.AssertExpectations(t)
}

func TestAuthHandler_ForgotPassword_MissingFields(t *testing.T) {
	mockAuth := new(mocks.MockAuthService)
	mockPwReset := new(mocks.MockPasswordResetService)
	h := handler.NewAuthHandler(mockAuth, nil, mockPwReset, nil, nil)

	// Missing email
	body, _ := json.Marshal(map[string]string{
		"tenant_slug": "test-tenant",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/forgot-password", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.ForgotPassword(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuthHandler_ForgotPassword_AlwaysReturns200(t *testing.T) {
	mockAuth := new(mocks.MockAuthService)
	mockPwReset := new(mocks.MockPasswordResetService)
	h := handler.NewAuthHandler(mockAuth, nil, mockPwReset, nil, nil)

	// Service returns nil even for non-existent user (by design)
	mockPwReset.On("ForgotPassword", mock.Anything, mock.AnythingOfType("service.ForgotPasswordInput")).Return(nil)

	body, _ := json.Marshal(map[string]string{
		"tenant_slug": "test-tenant",
		"email":       "nonexistent@test.com",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/forgot-password", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.ForgotPassword(c)

	assert.Equal(t, http.StatusOK, w.Code)
	mockPwReset.AssertExpectations(t)
}

func TestAuthHandler_ResetPassword_Success(t *testing.T) {
	mockAuth := new(mocks.MockAuthService)
	mockPwReset := new(mocks.MockPasswordResetService)
	h := handler.NewAuthHandler(mockAuth, nil, mockPwReset, nil, nil)

	mockPwReset.On("ResetPassword", mock.Anything, service.ResetPasswordInput{
		Token:       "valid-reset-token",
		NewPassword: "newpassword123",
	}).Return(nil)

	body, _ := json.Marshal(map[string]string{
		"token":        "valid-reset-token",
		"new_password": "newpassword123",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.ResetPassword(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp handler.APIResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.True(t, resp.Success)
	mockPwReset.AssertExpectations(t)
}

func TestAuthHandler_ResetPassword_MissingFields(t *testing.T) {
	mockAuth := new(mocks.MockAuthService)
	mockPwReset := new(mocks.MockPasswordResetService)
	h := handler.NewAuthHandler(mockAuth, nil, mockPwReset, nil, nil)

	// Missing new_password
	body, _ := json.Marshal(map[string]string{
		"token": "some-token",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.ResetPassword(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuthHandler_ResetPassword_InvalidToken(t *testing.T) {
	mockAuth := new(mocks.MockAuthService)
	mockPwReset := new(mocks.MockPasswordResetService)
	h := handler.NewAuthHandler(mockAuth, nil, mockPwReset, nil, nil)

	mockPwReset.On("ResetPassword", mock.Anything, mock.AnythingOfType("service.ResetPasswordInput")).
		Return(domain.ErrPasswordResetTokenInvalid)

	body, _ := json.Marshal(map[string]string{
		"token":        "invalid-token",
		"new_password": "newpassword123",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.ResetPassword(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	mockPwReset.AssertExpectations(t)
}

func TestAuthHandler_ForgotPassword_Disabled(t *testing.T) {
	mockAuth := new(mocks.MockAuthService)
	h := handler.NewAuthHandler(mockAuth, nil, nil, nil, nil) // nil password reset service

	body, _ := json.Marshal(map[string]string{
		"tenant_slug": "test-tenant",
		"email":       "user@test.com",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/forgot-password", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.ForgotPassword(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
