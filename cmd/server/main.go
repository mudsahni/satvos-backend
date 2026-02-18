package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"satvos/internal/config"
	"satvos/internal/domain"
	"satvos/internal/email/noop"
	"satvos/internal/email/ses"
	"satvos/internal/handler"
	"satvos/internal/parser"
	claudeparser "satvos/internal/parser/claude"
	geminiparser "satvos/internal/parser/gemini"
	openaiparser "satvos/internal/parser/openai"
	"satvos/internal/port"
	"satvos/internal/repository/postgres"
	"satvos/internal/router"
	"satvos/internal/service"
	s3storage "satvos/internal/storage/s3"
	"satvos/internal/validator"
	"satvos/internal/validator/invoice"

	googleauth "satvos/internal/auth/google"

	_ "satvos/docs" // swagger docs
)

// @title SATVOS API
// @version 1.0
// @description Multi-tenant document processing service with AI-powered invoice parsing and GST validation.
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.email support@satvos.io

// @license.name MIT
// @license.url https://opensource.org/licenses/MIT

// @host localhost:8080
// @BasePath /api/v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and the JWT token. Example: Bearer eyJhbGci...

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	log.SetOutput(os.Stdout)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.Server.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	db, err := postgres.NewDB(&cfg.DB)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Initialize repositories
	tenantRepo := postgres.NewTenantRepo(db)
	userRepo := postgres.NewUserRepo(db)
	fileRepo := postgres.NewFileMetaRepo(db)
	collectionRepo := postgres.NewCollectionRepo(db)
	collectionPermRepo := postgres.NewCollectionPermissionRepo(db)
	collectionFileRepo := postgres.NewCollectionFileRepo(db)

	// Initialize storage
	s3Client, err := s3storage.NewS3Client(&cfg.S3)
	if err != nil {
		return fmt.Errorf("failed to initialize S3 client: %w", err)
	}

	// Initialize document repositories
	docRepo := postgres.NewDocumentRepo(db)
	documentTagRepo := postgres.NewDocumentTagRepo(db)
	auditRepo := postgres.NewDocumentAuditRepo(db)
	summaryRepo := postgres.NewDocumentSummaryRepo(db)
	validationRuleRepo := postgres.NewDocumentValidationRuleRepo(db)
	statsRepo := postgres.NewStatsRepo(db)
	hsnRepo := postgres.NewHSNRepo(db)
	duplicateFinder := postgres.NewDuplicateFinderRepo(db)

	// Register parser providers
	parser.RegisterProvider("claude", func(provCfg *config.ParserProviderConfig) (port.DocumentParser, error) {
		return claudeparser.NewParser(provCfg), nil
	})
	parser.RegisterProvider("gemini", func(provCfg *config.ParserProviderConfig) (port.DocumentParser, error) {
		return geminiparser.NewParser(provCfg), nil
	})
	parser.RegisterProvider("openai", func(provCfg *config.ParserProviderConfig) (port.DocumentParser, error) {
		return openaiparser.NewParser(provCfg), nil
	})

	// Initialize primary parser
	primaryCfg := cfg.Parser.PrimaryConfig()
	primaryParser, err := parser.NewParser(primaryCfg)
	if err != nil {
		return fmt.Errorf("failed to create primary parser: %w", err)
	}

	// Build optional secondary and tertiary parsers
	var secondaryParser port.DocumentParser
	secondaryCfg := cfg.Parser.SecondaryConfig()
	if secondaryCfg != nil {
		sp, secErr := parser.NewParser(secondaryCfg)
		if secErr != nil {
			log.Printf("WARNING: failed to create secondary parser (%v)", secErr)
		} else {
			secondaryParser = sp
		}
	}

	var tertiaryParser port.DocumentParser
	tertiaryCfg := cfg.Parser.TertiaryConfig()
	if tertiaryCfg != nil {
		tp, terErr := parser.NewParser(tertiaryCfg)
		if terErr != nil {
			log.Printf("WARNING: failed to create tertiary parser (%v)", terErr)
		} else {
			tertiaryParser = tp
		}
	}

	// Wrap single-parse path in FallbackParser if extra parsers are available
	documentParser := buildFallbackParser(primaryParser, primaryCfg.Provider, secondaryParser, secondaryCfg, tertiaryParser, tertiaryCfg)

	// Initialize optional merge parser for dual-parse mode
	var mergeDocParser port.DocumentParser
	if secondaryParser != nil {
		primarySide := buildFallbackParser(primaryParser, primaryCfg.Provider, tertiaryParser, tertiaryCfg, nil, nil)
		secondarySide := buildFallbackParser(secondaryParser, secondaryCfg.Provider, tertiaryParser, tertiaryCfg, nil, nil)
		mergeDocParser = parser.NewMergeParser(primarySide, secondarySide)
		log.Printf("Multi-parser mode enabled: primary=%s, secondary=%s", primaryCfg.Provider, secondaryCfg.Provider)
	}

	// Initialize validation engine
	registry := validator.NewRegistry()
	for _, v := range invoice.AllBuiltinValidators() {
		registry.Register(v)
	}

	// Load HSN codes and register HSN validators
	hsnEntries, err := hsnRepo.LoadAll(context.Background())
	if err != nil {
		return fmt.Errorf("failed to load HSN codes: %w", err)
	}
	log.Printf("Loaded %d HSN code entries", len(hsnEntries))
	hsnLookup := invoice.NewHSNLookup(hsnEntries)
	for _, v := range invoice.HSNValidators(hsnLookup) {
		registry.Register(v)
	}

	// Register duplicate invoice validator
	registry.Register(invoice.DuplicateInvoiceValidator(duplicateFinder))

	validationEngine := validator.NewEngine(registry, validationRuleRepo, docRepo)

	// Initialize services
	authSvc := service.NewAuthService(userRepo, tenantRepo, cfg.JWT)
	fileSvc := service.NewFileService(fileRepo, s3Client, &cfg.S3)
	tenantSvc := service.NewTenantService(tenantRepo)
	userSvc := service.NewUserService(userRepo)
	collectionSvc := service.NewCollectionService(collectionRepo, collectionPermRepo, collectionFileRepo, fileSvc, userRepo)
	statsSvc := service.NewStatsService(statsRepo)
	reportRepo := postgres.NewReportRepo(db)
	reportSvc := service.NewReportService(reportRepo)

	// Initialize service account repositories and service
	saRepo := postgres.NewServiceAccountRepo(db)
	saPermRepo := postgres.NewServiceAccountPermissionRepo(db)
	serviceAccountSvc := service.NewServiceAccountService(saRepo, saPermRepo, collectionRepo)

	documentSvc := service.NewDocumentService(&service.DocumentServiceDeps{
		DocRepo:     docRepo,
		FileRepo:    fileRepo,
		UserRepo:    userRepo,
		PermRepo:    collectionPermRepo,
		TagRepo:     documentTagRepo,
		AuditRepo:   auditRepo,
		SummaryRepo: summaryRepo,
		SAPermRepo:  saPermRepo,
		Parser:      documentParser,
		MergeParser: mergeDocParser, // nil when single-parse mode
		Storage:     s3Client,
		Validator:   validationEngine,
	})

	// Auto-create free tier tenant if it doesn't exist
	if _, ftErr := tenantRepo.GetBySlug(context.Background(), cfg.FreeTier.TenantSlug); ftErr != nil {
		log.Printf("Free tier tenant '%s' not found, creating...", cfg.FreeTier.TenantSlug)
		ft := &domain.Tenant{
			Name:     "SATVOS Free Tier",
			Slug:     cfg.FreeTier.TenantSlug,
			IsActive: true,
		}
		if createErr := tenantRepo.Create(context.Background(), ft); createErr != nil {
			log.Printf("WARNING: failed to create free tier tenant: %v", createErr)
		} else {
			log.Printf("Free tier tenant '%s' created with ID %s", cfg.FreeTier.TenantSlug, ft.ID)
		}
	} else {
		log.Printf("Free tier tenant '%s' ready", cfg.FreeTier.TenantSlug)
	}

	// Initialize email sender
	var emailSender port.EmailSender
	switch cfg.Email.Provider {
	case "ses":
		emailSender, err = ses.NewSESSender(cfg.Email.Region, cfg.Email.FromAddress, cfg.Email.FromName, cfg.Email.FrontendURL, cfg.Email.AccessKey, cfg.Email.SecretKey)
		if err != nil {
			return fmt.Errorf("failed to initialize SES email sender: %w", err)
		}
		log.Println("Email sender: AWS SES")
	default:
		emailSender = noop.NewNoopSender(cfg.Email.FrontendURL)
		log.Println("Email sender: noop (verification URLs logged to stdout)")
	}

	registrationSvc := service.NewRegistrationService(tenantRepo, userRepo, collectionRepo, collectionPermRepo, authSvc, emailSender, cfg.JWT, cfg.FreeTier)
	passwordResetSvc := service.NewPasswordResetService(tenantRepo, userRepo, emailSender, cfg.JWT)

	// Initialize social auth (optional — disabled if no client ID configured)
	var socialAuthSvc service.SocialAuthService
	if cfg.GoogleAuth.ClientID != "" {
		googleVerifier := googleauth.NewVerifier(cfg.GoogleAuth.ClientID)
		verifiers := map[string]port.SocialTokenVerifier{
			string(domain.AuthProviderGoogle): googleVerifier,
		}
		socialAuthSvc = service.NewSocialAuthService(
			verifiers, tenantRepo, userRepo, collectionRepo, collectionPermRepo, authSvc, cfg.FreeTier,
		)
		log.Println("Social auth enabled: Google")
	}

	// Start parse queue worker
	queueCfg := service.ParseQueueConfig{
		PollInterval:       time.Duration(cfg.Queue.PollIntervalSecs) * time.Second,
		MaxRetries:         cfg.Queue.MaxRetries,
		Concurrency:        cfg.Queue.Concurrency,
		StaleThreshold:     10 * time.Minute,
		StaleSweepInterval: 5 * time.Minute,
	}
	queueWorker := service.NewParseQueueWorker(docRepo, documentSvc, queueCfg)
	queueCtx, queueStop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer queueStop()
	go queueWorker.Start(queueCtx)

	// Initialize handlers
	authH := handler.NewAuthHandler(authSvc, registrationSvc, passwordResetSvc, socialAuthSvc)
	fileH := handler.NewFileHandler(fileSvc, collectionSvc)
	tenantH := handler.NewTenantHandler(tenantSvc)
	userH := handler.NewUserHandler(userSvc)
	healthH := handler.NewHealthHandler(db)
	collectionH := handler.NewCollectionHandler(collectionSvc, documentSvc)
	documentH := handler.NewDocumentHandler(documentSvc, auditRepo)
	statsH := handler.NewStatsHandler(statsSvc)
	reportH := handler.NewReportHandler(reportSvc)
	serviceAccountH := handler.NewServiceAccountHandler(serviceAccountSvc)

	// Setup router
	r := router.Setup(authSvc, authH, fileH, tenantH, userH, healthH, collectionH, documentH, statsH, reportH, serviceAccountH, cfg.CORS.AllowedOrigins, userRepo, serviceAccountSvc)

	log.Printf("Server starting on %s", cfg.Server.Port)
	if err := r.Run(cfg.Server.Port); err != nil {
		return fmt.Errorf("server failed: %w", err)
	}

	return nil
}

// buildFallbackParser wraps a primary parser with optional fallback parsers.
// If no fallbacks are available, returns the primary parser directly.
func buildFallbackParser(p1 port.DocumentParser, p1Name string, p2 port.DocumentParser, p2Cfg *config.ParserProviderConfig, p3 port.DocumentParser, p3Cfg *config.ParserProviderConfig) port.DocumentParser {
	parsers := []port.DocumentParser{p1}
	names := []string{p1Name}

	if p2 != nil {
		parsers = append(parsers, p2)
		names = append(names, p2Cfg.Provider)
	}
	if p3 != nil {
		parsers = append(parsers, p3)
		names = append(names, p3Cfg.Provider)
	}

	if len(parsers) == 1 {
		return p1
	}
	return parser.NewFallbackParser(parsers, names)
}
