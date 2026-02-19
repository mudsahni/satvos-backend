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
	"github.com/google/uuid"

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
	docRepo := postgres.NewDocumentRepo(db)
	documentTagRepo := postgres.NewDocumentTagRepo(db)
	auditRepo := postgres.NewDocumentAuditRepo(db)
	summaryRepo := postgres.NewDocumentSummaryRepo(db)
	validationRuleRepo := postgres.NewDocumentValidationRuleRepo(db)
	statsRepo := postgres.NewStatsRepo(db)
	hsnRepo := postgres.NewHSNRepo(db)
	duplicateFinder := postgres.NewDuplicateFinderRepo(db)
	reportRepo := postgres.NewReportRepo(db)
	saRepo := postgres.NewServiceAccountRepo(db)
	saPermRepo := postgres.NewServiceAccountPermissionRepo(db)

	// Initialize storage
	s3Client, err := s3storage.NewS3Client(&cfg.S3)
	if err != nil {
		return fmt.Errorf("failed to initialize S3 client: %w", err)
	}

	// Initialize parsers
	documentParser, mergeDocParser, err := initParsers(cfg)
	if err != nil {
		return err
	}

	// Initialize validation engine
	validationEngine, err := initValidationEngine(docRepo, validationRuleRepo, hsnRepo, duplicateFinder)
	if err != nil {
		return err
	}

	// Initialize services
	authSvc := service.NewAuthService(userRepo, tenantRepo, cfg.JWT)
	fileSvc := service.NewFileService(fileRepo, s3Client, &cfg.S3)
	serviceAccountSvc := service.NewServiceAccountService(saRepo, saPermRepo, collectionRepo)
	tenantSvc := service.NewTenantService(tenantRepo, serviceAccountSvc)
	userSvc := service.NewUserService(userRepo)
	collectionSvc := service.NewCollectionService(collectionRepo, collectionPermRepo, collectionFileRepo, fileSvc, userRepo, saPermRepo)
	statsSvc := service.NewStatsService(statsRepo)
	reportSvc := service.NewReportService(reportRepo)

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
	ensureFreeTierTenant(tenantRepo, cfg.FreeTier.TenantSlug, serviceAccountSvc)

	// Initialize email sender
	emailSender, err := initEmailSender(cfg)
	if err != nil {
		return err
	}

	registrationSvc := service.NewRegistrationService(tenantRepo, userRepo, collectionRepo, collectionPermRepo, authSvc, emailSender, cfg.JWT, cfg.FreeTier)
	passwordResetSvc := service.NewPasswordResetService(tenantRepo, userRepo, emailSender, cfg.JWT)
	socialAuthSvc := initSocialAuth(cfg, tenantRepo, userRepo, collectionRepo, collectionPermRepo, authSvc)

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

	// Setup router and start server
	r := router.Setup(authSvc, authH, fileH, tenantH, userH, healthH, collectionH, documentH, statsH, reportH, serviceAccountH, cfg.CORS.AllowedOrigins, userRepo, serviceAccountSvc)

	log.Printf("Server starting on %s", cfg.Server.Port)
	if err := r.Run(cfg.Server.Port); err != nil {
		return fmt.Errorf("server failed: %w", err)
	}

	return nil
}

// initParsers registers parser providers and builds the single-parse and optional merge parser.
func initParsers(cfg *config.Config) (singleParser, mergeParser port.DocumentParser, err error) {
	parser.RegisterProvider("claude", func(provCfg *config.ParserProviderConfig) (port.DocumentParser, error) {
		return claudeparser.NewParser(provCfg), nil
	})
	parser.RegisterProvider("gemini", func(provCfg *config.ParserProviderConfig) (port.DocumentParser, error) {
		return geminiparser.NewParser(provCfg), nil
	})
	parser.RegisterProvider("openai", func(provCfg *config.ParserProviderConfig) (port.DocumentParser, error) {
		return openaiparser.NewParser(provCfg), nil
	})

	primaryCfg := cfg.Parser.PrimaryConfig()
	primaryParser, err := parser.NewParser(primaryCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create primary parser: %w", err)
	}

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

	documentParser := buildFallbackParser(primaryParser, primaryCfg.Provider, secondaryParser, secondaryCfg, tertiaryParser, tertiaryCfg)

	var mergeDocParser port.DocumentParser
	if secondaryParser != nil {
		primarySide := buildFallbackParser(primaryParser, primaryCfg.Provider, tertiaryParser, tertiaryCfg, nil, nil)
		secondarySide := buildFallbackParser(secondaryParser, secondaryCfg.Provider, tertiaryParser, tertiaryCfg, nil, nil)
		mergeDocParser = parser.NewMergeParser(primarySide, secondarySide)
		log.Printf("Multi-parser mode enabled: primary=%s, secondary=%s", primaryCfg.Provider, secondaryCfg.Provider)
	}

	return documentParser, mergeDocParser, nil
}

// initValidationEngine creates the validation engine with all builtin, HSN, and duplicate validators.
func initValidationEngine(
	docRepo port.DocumentRepository,
	ruleRepo port.DocumentValidationRuleRepository,
	hsnRepo port.HSNRepository,
	dupFinder port.DuplicateInvoiceFinder,
) (*validator.Engine, error) {
	registry := validator.NewRegistry()
	for _, v := range invoice.AllBuiltinValidators() {
		registry.Register(v)
	}

	hsnEntries, err := hsnRepo.LoadAll(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to load HSN codes: %w", err)
	}
	log.Printf("Loaded %d HSN code entries", len(hsnEntries))
	hsnLookup := invoice.NewHSNLookup(hsnEntries)
	for _, v := range invoice.HSNValidators(hsnLookup) {
		registry.Register(v)
	}

	registry.Register(invoice.DuplicateInvoiceValidator(dupFinder))

	return validator.NewEngine(registry, ruleRepo, docRepo), nil
}

// initEmailSender creates the appropriate email sender based on config.
func initEmailSender(cfg *config.Config) (port.EmailSender, error) {
	switch cfg.Email.Provider {
	case "ses":
		sender, err := ses.NewSESSender(cfg.Email.Region, cfg.Email.FromAddress, cfg.Email.FromName, cfg.Email.FrontendURL, cfg.Email.AccessKey, cfg.Email.SecretKey)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize SES email sender: %w", err)
		}
		log.Println("Email sender: AWS SES")
		return sender, nil
	default:
		log.Println("Email sender: noop (verification URLs logged to stdout)")
		return noop.NewNoopSender(cfg.Email.FrontendURL), nil
	}
}

// initSocialAuth creates the social auth service if Google auth is configured.
func initSocialAuth(
	cfg *config.Config,
	tenantRepo port.TenantRepository,
	userRepo port.UserRepository,
	collectionRepo port.CollectionRepository,
	collectionPermRepo port.CollectionPermissionRepository,
	authSvc service.AuthService,
) service.SocialAuthService {
	if cfg.GoogleAuth.ClientID == "" {
		return nil
	}
	googleVerifier := googleauth.NewVerifier(cfg.GoogleAuth.ClientID)
	verifiers := map[string]port.SocialTokenVerifier{
		string(domain.AuthProviderGoogle): googleVerifier,
	}
	svc := service.NewSocialAuthService(
		verifiers, tenantRepo, userRepo, collectionRepo, collectionPermRepo, authSvc, cfg.FreeTier,
	)
	log.Println("Social auth enabled: Google")
	return svc
}

// ensureFreeTierTenant auto-creates the free tier tenant if it doesn't exist.
func ensureFreeTierTenant(tenantRepo port.TenantRepository, slug string, saSvc service.ServiceAccountService) {
	tenant, err := tenantRepo.GetBySlug(context.Background(), slug)
	if err != nil {
		log.Printf("Free tier tenant '%s' not found, creating...", slug)
		tenant = &domain.Tenant{
			Name:     "SATVOS Free Tier",
			Slug:     slug,
			IsActive: true,
		}
		if createErr := tenantRepo.Create(context.Background(), tenant); createErr != nil {
			log.Printf("WARNING: failed to create free tier tenant: %v", createErr)
			return
		}
		log.Printf("Free tier tenant '%s' created with ID %s", slug, tenant.ID)
	} else {
		log.Printf("Free tier tenant '%s' ready", slug)
	}

	// Ensure inbound_email SA exists (idempotent)
	if saSvc != nil {
		// createdBy=uuid.Nil signals system-created (no FK constraint on created_by)
		apiKey, saErr := saSvc.EnsureInboundEmailServiceAccount(context.Background(), tenant.ID, uuid.Nil)
		if saErr != nil {
			log.Printf("WARNING: failed to ensure inbound_email SA: %v", saErr)
		} else if apiKey != "" {
			log.Printf("*** INBOUND EMAIL API KEY for tenant '%s': %s...%s ***", slug, apiKey[:11], apiKey[len(apiKey)-4:])
			log.Printf("*** Copy this key to DynamoDB — it will not be shown again ***")
		}
	}
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
