package handler

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"satvos/internal/service"
)

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	authService          service.AuthService
	registrationService  service.RegistrationService
	passwordResetService service.PasswordResetService
	socialAuthService    service.SocialAuthService
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(authService service.AuthService, registrationService service.RegistrationService, passwordResetService service.PasswordResetService, socialAuthService service.SocialAuthService) *AuthHandler {
	return &AuthHandler{authService: authService, registrationService: registrationService, passwordResetService: passwordResetService, socialAuthService: socialAuthService}
}

// Login handles POST /api/v1/auth/login
// @Summary Login
// @Description Authenticate with email and password, returns JWT token pair
// @Tags auth
// @Accept json
// @Produce json
// @Param request body LoginRequest true "Login credentials"
// @Success 200 {object} Response{data=TokenResponse} "Token pair"
// @Failure 400 {object} ErrorResponseBody "Validation error"
// @Failure 401 {object} ErrorResponseBody "Invalid credentials"
// @Router /auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var input service.LoginInput
	if err := c.ShouldBindJSON(&input); err != nil {
		RespondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	tokenPair, err := h.authService.Login(c.Request.Context(), input)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, tokenPair)
}

// RefreshToken handles POST /api/v1/auth/refresh
// @Summary Refresh token
// @Description Exchange a refresh token for a new JWT token pair
// @Tags auth
// @Accept json
// @Produce json
// @Param request body RefreshRequest true "Refresh token"
// @Success 200 {object} Response{data=TokenResponse} "New token pair"
// @Failure 400 {object} ErrorResponseBody "Validation error"
// @Failure 401 {object} ErrorResponseBody "Invalid or expired refresh token"
// @Router /auth/refresh [post]
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var input service.RefreshInput
	if err := c.ShouldBindJSON(&input); err != nil {
		RespondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	tokenPair, err := h.authService.RefreshToken(c.Request.Context(), input.RefreshToken)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, tokenPair)
}

// Register handles POST /api/v1/auth/register
// @Summary Register free-tier account
// @Description Self-register a free-tier account with email verification
// @Tags auth
// @Accept json
// @Produce json
// @Param request body RegisterRequest true "Registration details"
// @Success 201 {object} Response{data=RegisterResponse} "Account created"
// @Failure 400 {object} ErrorResponseBody "Validation error"
// @Failure 404 {object} ErrorResponseBody "Registration not enabled"
// @Failure 409 {object} ErrorResponseBody "Email already exists"
// @Router /auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	if h.registrationService == nil {
		RespondError(c, http.StatusNotFound, "NOT_FOUND", "registration is not enabled")
		return
	}

	var input service.RegisterInput
	if err := c.ShouldBindJSON(&input); err != nil {
		RespondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	output, err := h.registrationService.Register(c.Request.Context(), input)
	if err != nil {
		HandleError(c, err)
		return
	}

	RespondCreated(c, output)
}

// VerifyEmail handles GET /api/v1/auth/verify-email?token=...
// @Summary Verify email
// @Description Verify email address using the token from the verification email
// @Tags auth
// @Produce json
// @Param token query string true "Email verification JWT token"
// @Success 200 {object} Response{data=MessageResponse} "Email verified"
// @Failure 400 {object} ErrorResponseBody "Missing or invalid token"
// @Failure 401 {object} ErrorResponseBody "Token expired"
// @Failure 404 {object} ErrorResponseBody "Registration not enabled"
// @Router /auth/verify-email [get]
func (h *AuthHandler) VerifyEmail(c *gin.Context) {
	if h.registrationService == nil {
		RespondError(c, http.StatusNotFound, "NOT_FOUND", "registration is not enabled")
		return
	}

	token := c.Query("token")
	if token == "" {
		RespondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "token query parameter is required")
		return
	}

	if err := h.registrationService.VerifyEmail(c.Request.Context(), token); err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, gin.H{"message": "email verified successfully"})
}

// ForgotPassword handles POST /api/v1/auth/forgot-password
// @Summary Request password reset
// @Description Send a password reset email. Always returns 200 to prevent email enumeration.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body ForgotPasswordRequest true "Account identifier"
// @Success 200 {object} Response{data=MessageResponse} "Reset email sent (if account exists)"
// @Failure 400 {object} ErrorResponseBody "Validation error"
// @Failure 404 {object} ErrorResponseBody "Password reset not enabled"
// @Router /auth/forgot-password [post]
func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	if h.passwordResetService == nil {
		RespondError(c, http.StatusNotFound, "NOT_FOUND", "password reset is not enabled")
		return
	}

	var input service.ForgotPasswordInput
	if err := c.ShouldBindJSON(&input); err != nil {
		RespondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	if err := h.passwordResetService.ForgotPassword(c.Request.Context(), input); err != nil {
		// Never leak information — always return 200
		log.Printf("forgot-password internal error: %v", err)
	}

	RespondOK(c, gin.H{"message": "if an account with that email exists, a password reset link has been sent"})
}

// ResetPassword handles POST /api/v1/auth/reset-password
// @Summary Reset password
// @Description Reset password using the token from the reset email. Token is single-use.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body ResetPasswordRequest true "Reset token and new password"
// @Success 200 {object} Response{data=MessageResponse} "Password reset"
// @Failure 400 {object} ErrorResponseBody "Validation error or invalid token"
// @Failure 404 {object} ErrorResponseBody "Password reset not enabled"
// @Router /auth/reset-password [post]
func (h *AuthHandler) ResetPassword(c *gin.Context) {
	if h.passwordResetService == nil {
		RespondError(c, http.StatusNotFound, "NOT_FOUND", "password reset is not enabled")
		return
	}

	var input service.ResetPasswordInput
	if err := c.ShouldBindJSON(&input); err != nil {
		RespondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	if err := h.passwordResetService.ResetPassword(c.Request.Context(), input); err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, gin.H{"message": "password has been reset successfully"})
}

// SocialLogin handles POST /api/v1/auth/social-login
// @Summary Social login
// @Description Authenticate via a social provider (Google). Auto-registers new users on the free tier.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body SocialLoginRequest true "Social provider and ID token"
// @Success 200 {object} Response{data=SocialLoginResponse} "Existing user logged in"
// @Success 201 {object} Response{data=SocialLoginResponse} "New user registered"
// @Failure 400 {object} ErrorResponseBody "Validation error or invalid token"
// @Failure 404 {object} ErrorResponseBody "Social login not enabled"
// @Router /auth/social-login [post]
func (h *AuthHandler) SocialLogin(c *gin.Context) {
	if h.socialAuthService == nil {
		RespondError(c, http.StatusNotFound, "NOT_FOUND", "social login is not enabled")
		return
	}

	var input service.SocialLoginInput
	if err := c.ShouldBindJSON(&input); err != nil {
		RespondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	output, err := h.socialAuthService.SocialLogin(c.Request.Context(), input)
	if err != nil {
		HandleError(c, err)
		return
	}

	if output.IsNewUser {
		RespondCreated(c, output)
	} else {
		RespondOK(c, output)
	}
}

// ResendVerification handles POST /api/v1/auth/resend-verification
// @Summary Resend verification email
// @Description Resend the email verification link. Requires authentication. No-op if already verified.
// @Tags auth
// @Produce json
// @Success 200 {object} Response{data=MessageResponse} "Verification email sent"
// @Failure 401 {object} ErrorResponseBody "Unauthorized"
// @Failure 404 {object} ErrorResponseBody "Registration not enabled"
// @Security BearerAuth
// @Router /auth/resend-verification [post]
func (h *AuthHandler) ResendVerification(c *gin.Context) {
	if h.registrationService == nil {
		RespondError(c, http.StatusNotFound, "NOT_FOUND", "registration is not enabled")
		return
	}

	tenantID, userID, _, ok := extractAuthContext(c)
	if !ok {
		return
	}

	if err := h.registrationService.ResendVerification(c.Request.Context(), tenantID, userID); err != nil {
		HandleError(c, err)
		return
	}

	RespondOK(c, gin.H{"message": "verification email sent"})
}
