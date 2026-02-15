package router

import (
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"satvos/internal/domain"
	"satvos/internal/handler"
	"satvos/internal/middleware"
	"satvos/internal/port"
	"satvos/internal/service"
)

// Setup configures the Gin engine with all routes and middleware.
func Setup(
	authSvc service.AuthService,
	authH *handler.AuthHandler,
	fileH *handler.FileHandler,
	tenantH *handler.TenantHandler,
	userH *handler.UserHandler,
	healthH *handler.HealthHandler,
	collectionH *handler.CollectionHandler,
	documentH *handler.DocumentHandler,
	statsH *handler.StatsHandler,
	reportH *handler.ReportHandler,
	corsOrigins []string,
	userRepo port.UserRepository,
) *gin.Engine {
	r := gin.New()

	// Global middleware
	r.Use(middleware.Recovery())
	r.Use(middleware.RequestID())
	r.Use(middleware.CORS(corsOrigins))
	r.Use(middleware.Logger())

	// Health checks
	r.GET("/healthz", healthH.Liveness)
	r.GET("/readyz", healthH.Readiness)

	// Swagger docs
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler,
		ginSwagger.PersistAuthorization(true),
	))

	v1 := r.Group("/api/v1")

	// Public auth routes
	auth := v1.Group("/auth")
	auth.POST("/login", authH.Login)
	auth.POST("/refresh", authH.RefreshToken)
	auth.POST("/register", authH.Register)
	auth.GET("/verify-email", authH.VerifyEmail)
	auth.POST("/forgot-password", authH.ForgotPassword)
	auth.POST("/reset-password", authH.ResetPassword)
	auth.POST("/social-login", authH.SocialLogin)

	// Protected routes - require valid JWT
	protected := v1.Group("")
	protected.Use(middleware.AuthMiddleware(authSvc))

	// Resend verification (authenticated, no email verification required)
	protected.POST("/auth/resend-verification", authH.ResendVerification)

	// File routes
	files := protected.Group("/files")
	files.POST("/upload",
		middleware.RequireRole(domain.RoleAdmin, domain.RoleManager, domain.RoleMember, domain.RoleFree),
		middleware.RequireEmailVerified(userRepo),
		fileH.Upload)
	files.GET("", fileH.List)
	files.GET("/:id", fileH.GetByID)
	files.DELETE("/:id", middleware.RequireRole(domain.RoleAdmin), fileH.Delete)

	// Collection routes
	collections := protected.Group("/collections")
	collections.POST("", middleware.RequireRole(domain.RoleAdmin, domain.RoleManager, domain.RoleMember), collectionH.Create)
	collections.GET("", collectionH.List)
	collections.GET("/:id", collectionH.GetByID)
	collections.PUT("/:id", collectionH.Update)
	collections.DELETE("/:id", collectionH.Delete)
	collections.POST("/:id/files", collectionH.BatchUploadFiles)
	collections.DELETE("/:id/files/:fileId", collectionH.RemoveFile)
	collections.POST("/:id/permissions", collectionH.SetPermission)
	collections.GET("/:id/permissions", collectionH.ListPermissions)
	collections.DELETE("/:id/permissions/:userId", collectionH.RemovePermission)
	collections.GET("/:id/export/csv", collectionH.ExportCSV)
	collections.GET("/:id/export/tally", collectionH.ExportTally)

	// Document routes
	documents := protected.Group("/documents")
	documents.POST("", middleware.RequireEmailVerified(userRepo), documentH.Create)
	documents.GET("", documentH.List)
	documents.GET("/search/tags", documentH.SearchByTag)
	documents.GET("/review-queue", documentH.ReviewQueue)
	documents.GET("/:id", documentH.GetByID)
	documents.PUT("/:id", documentH.EditStructuredData)
	documents.POST("/:id/retry", documentH.Retry)
	documents.PUT("/:id/review", documentH.UpdateReview)
	documents.PUT("/:id/assign", documentH.AssignDocument)
	documents.PUT("/:id/structured-data", documentH.EditStructuredData)
	documents.POST("/:id/validate", documentH.Validate)
	documents.GET("/:id/validation", documentH.GetValidation)
	documents.GET("/:id/tags", documentH.ListTags)
	documents.POST("/:id/tags", documentH.AddTags)
	documents.DELETE("/:id/tags/:tagId", documentH.DeleteTag)
	documents.GET("/:id/audit", documentH.ListAudit)
	documents.DELETE("/:id", middleware.RequireRole(domain.RoleAdmin), documentH.Delete)

	// Stats
	protected.GET("/stats", statsH.GetStats)

	// Report routes
	reports := protected.Group("/reports")
	reports.GET("/sellers", reportH.Sellers)
	reports.GET("/buyers", reportH.Buyers)
	reports.GET("/party-ledger", reportH.PartyLedger)
	reports.GET("/financial-summary", reportH.FinancialSummary)
	reports.GET("/tax-summary", reportH.TaxSummary)
	reports.GET("/hsn-summary", reportH.HSNSummary)
	reports.GET("/collections-overview", reportH.CollectionsOverview)

	// User management (tenant-scoped)
	users := protected.Group("/users")
	users.POST("", middleware.RequireRole(domain.RoleAdmin), userH.Create)
	users.GET("", middleware.RequireRole(domain.RoleAdmin), userH.List)
	users.GET("/:id", userH.GetByID)
	users.PUT("/:id", userH.Update)
	users.DELETE("/:id", middleware.RequireRole(domain.RoleAdmin), userH.Delete)

	// Admin routes - tenant management
	admin := v1.Group("/admin")
	admin.Use(middleware.AuthMiddleware(authSvc))
	admin.Use(middleware.RequireRole(domain.RoleAdmin))
	admin.POST("/tenants", tenantH.Create)
	admin.GET("/tenants", tenantH.List)
	admin.GET("/tenants/:id", tenantH.GetByID)
	admin.PUT("/tenants/:id", tenantH.Update)
	admin.DELETE("/tenants/:id", tenantH.Delete)

	return r
}
