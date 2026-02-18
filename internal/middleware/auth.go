package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"satvos/internal/domain"
	"satvos/internal/port"
	"satvos/internal/service"
)

const (
	ContextKeyTenantID          = "tenant_id"
	ContextKeyUserID            = "user_id"
	ContextKeyEmail             = "email"
	ContextKeyRole              = "role"
	ContextKeyClaims            = "claims"
	ContextKeyServiceAccountID  = "service_account_id"
	ContextKeyIsServiceAccount  = "is_service_account"
)

// AuthMiddleware returns Gin middleware that validates JWT tokens (or API keys for
// service accounts) and injects tenant and user context.
func AuthMiddleware(authService service.AuthService, saSvc ...service.ServiceAccountService) gin.HandlerFunc {
	var serviceAccountSvc service.ServiceAccountService
	if len(saSvc) > 0 {
		serviceAccountSvc = saSvc[0]
	}

	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   gin.H{"code": "UNAUTHORIZED", "message": "missing or invalid authorization header"},
			})
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")

		// API key path: tokens starting with "sk_" are service account keys
		if strings.HasPrefix(token, "sk_") && serviceAccountSvc != nil {
			sa, err := serviceAccountSvc.Authenticate(c.Request.Context(), token)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"success": false,
					"error":   gin.H{"code": "UNAUTHORIZED", "message": "invalid or expired API key"},
				})
				return
			}

			c.Set(ContextKeyTenantID, sa.TenantID)
			c.Set(ContextKeyUserID, sa.ID) // service account ID used as user ID
			c.Set(ContextKeyEmail, "")
			c.Set(ContextKeyRole, string(domain.RoleService))
			c.Set(ContextKeyServiceAccountID, sa.ID)
			c.Set(ContextKeyIsServiceAccount, true)
			c.Next()
			return
		}

		// JWT path: standard user token
		claims, err := authService.ValidateToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   gin.H{"code": "UNAUTHORIZED", "message": "invalid or expired token"},
			})
			return
		}

		c.Set(ContextKeyTenantID, claims.TenantID)
		c.Set(ContextKeyUserID, claims.UserID)
		c.Set(ContextKeyEmail, claims.Email)
		c.Set(ContextKeyRole, string(claims.Role))
		c.Set(ContextKeyClaims, claims)
		c.Set(ContextKeyIsServiceAccount, false)
		c.Next()
	}
}

// IsServiceAccount returns true if the current request is authenticated via API key.
func IsServiceAccount(c *gin.Context) bool {
	val, exists := c.Get(ContextKeyIsServiceAccount)
	if !exists {
		return false
	}
	return val.(bool)
}

// GetServiceAccountID extracts the service account ID from the Gin context.
func GetServiceAccountID(c *gin.Context) (uuid.UUID, error) {
	val, exists := c.Get(ContextKeyServiceAccountID)
	if !exists {
		return uuid.Nil, domain.ErrUnauthorized
	}
	return val.(uuid.UUID), nil
}

// RequireRole returns middleware that checks the user's role against allowed roles.
func RequireRole(roles ...domain.UserRole) gin.HandlerFunc {
	return func(c *gin.Context) {
		roleStr, exists := c.Get(ContextKeyRole)
		if !exists {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   gin.H{"code": "FORBIDDEN", "message": "role not found in context"},
			})
			return
		}

		userRole := domain.UserRole(roleStr.(string))
		for _, r := range roles {
			if userRole == r {
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   gin.H{"code": "FORBIDDEN", "message": "insufficient permissions"},
		})
	}
}

// GetTenantID extracts the tenant ID from the Gin context.
func GetTenantID(c *gin.Context) (uuid.UUID, error) {
	val, exists := c.Get(ContextKeyTenantID)
	if !exists {
		return uuid.Nil, domain.ErrUnauthorized
	}
	return val.(uuid.UUID), nil
}

// GetUserID extracts the user ID from the Gin context.
func GetUserID(c *gin.Context) (uuid.UUID, error) {
	val, exists := c.Get(ContextKeyUserID)
	if !exists {
		return uuid.Nil, domain.ErrUnauthorized
	}
	return val.(uuid.UUID), nil
}

// GetRole extracts the user role string from the Gin context.
func GetRole(c *gin.Context) string {
	val, exists := c.Get(ContextKeyRole)
	if !exists {
		return ""
	}
	return val.(string)
}

// RequireEmailVerified returns middleware that blocks unverified free-tier users.
// Paid-tier users (admin, manager, member, viewer) skip verification check entirely.
func RequireEmailVerified(userRepo port.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		role := domain.UserRole(GetRole(c))
		if role != domain.RoleFree {
			c.Next()
			return
		}

		tenantID, err := GetTenantID(c)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   gin.H{"code": "UNAUTHORIZED", "message": "missing tenant context"},
			})
			return
		}
		userID, err := GetUserID(c)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   gin.H{"code": "UNAUTHORIZED", "message": "missing user context"},
			})
			return
		}

		user, err := userRepo.GetByID(c.Request.Context(), tenantID, userID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   gin.H{"code": "UNAUTHORIZED", "message": "user not found"},
			})
			return
		}

		if !user.EmailVerified {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   gin.H{"code": "EMAIL_NOT_VERIFIED", "message": "please verify your email before performing this action"},
			})
			return
		}

		c.Next()
	}
}
