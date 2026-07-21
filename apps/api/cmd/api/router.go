package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"lapangango-api/internal/admin"
	"lapangango-api/internal/analytics"
	"lapangango-api/internal/audit"
	"lapangango-api/internal/auth"
	"lapangango-api/internal/availability"
	"lapangango-api/internal/blockedslots"
	"lapangango-api/internal/bookings"
	"lapangango-api/internal/commercialterms"
	"lapangango-api/internal/config"
	"lapangango-api/internal/courts"
	"lapangango-api/internal/email"
	"lapangango-api/internal/finance"
	"lapangango-api/internal/mabar"
	"lapangango-api/internal/middleware"
	"lapangango-api/internal/notifications"
	"lapangango-api/internal/owneraccess"
	"lapangango-api/internal/owners"
	"lapangango-api/internal/platformfinance"
	"lapangango-api/internal/promos"
	"lapangango-api/internal/refunds"
	"lapangango-api/internal/schedules"
	"lapangango-api/internal/staff"
	"lapangango-api/internal/venues"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setupRouter(ctx context.Context, cfg config.Config, dbPool *pgxpool.Pool, startWorkers bool) (*gin.Engine, context.CancelFunc, error) {
	r := gin.Default()
	r.Use(middleware.CORS())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"message": "LapanganGo API is running",
		})
	})

	r.GET("/db-health", func(c *gin.Context) {
		err := dbPool.Ping(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  "error",
				"message": "Database not connected",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"message": "PostgreSQL connected",
		})
	})

	generalRateLimiter := middleware.NewRateLimiter(cfg.RedisURL, "general", cfg.GeneralRateLimitPerMinute, time.Minute)
	r.Use(generalRateLimiter)

	tokenService := auth.NewTokenService(cfg.JWTSecret, cfg.JWTExpiresInHours)
	authRepository := auth.NewRepository(dbPool)
	authService := auth.NewService(authRepository, tokenService)
	authHandler := auth.NewHandler(authService)
	authMiddleware := middleware.Auth(tokenService)
	requireActiveUser := middleware.RequireActiveUser(authRepository)

	authRateLimiter := middleware.NewRateLimiter(cfg.RedisURL, "auth", cfg.AuthRateLimitPerMinute, time.Minute)
	authHandler.RegisterRoutes(r, authMiddleware, requireActiveUser, authRateLimiter)

	availabilityRepository := availability.NewRepository(dbPool)
	availabilityService := availability.NewService(availabilityRepository)
	availabilityHandler := availability.NewHandler(availabilityService)
	availabilityHandler.RegisterRoutes(r)

	ownerAccessRepo := owneraccess.NewRepository(dbPool)
	ownerWorkspaceMiddleware := middleware.OwnerWorkspaceAccess(ownerAccessRepo)
	requireActualOwner := middleware.RequireActualOwner()

	ownerRepository := owners.NewRepository(dbPool)
	ownerService := owners.NewService(ownerRepository)
	ownerHandler := owners.NewHandler(ownerService)
	ownerHandler.RegisterRoutes(r, authMiddleware, requireActiveUser, ownerWorkspaceMiddleware, requireActualOwner)

	auditRepository := audit.NewRepository(dbPool)
	auditService := audit.NewService(auditRepository)

	var emailService email.Service
	if cfg.EmailDeliveryEnabled {
		emailService = email.NewSMTPService(cfg)
	} else {
		emailService = email.NewNoopService()
	}

	staffRepository := staff.NewRepository(dbPool)
	staffService := staff.NewService(staffRepository, emailService)
	staffHandler := staff.NewHandler(staffService, auditService)
	staffHandler.RegisterRoutes(r, authMiddleware, requireActiveUser, ownerWorkspaceMiddleware, requireActualOwner)

	venueRepository := venues.NewRepository(dbPool)
	venueService := venues.NewService(venueRepository)
	venueHandler := venues.NewHandler(venueService)
	venueHandler.RegisterRoutes(r, authMiddleware, requireActiveUser, ownerWorkspaceMiddleware)

	courtRepository := courts.NewRepository(dbPool)
	courtService := courts.NewService(courtRepository)
	courtHandler := courts.NewHandler(courtService)
	courtHandler.RegisterRoutes(r, authMiddleware, requireActiveUser, ownerWorkspaceMiddleware)

	scheduleRepository := schedules.NewRepository(dbPool)
	scheduleService := schedules.NewService(scheduleRepository)
	scheduleHandler := schedules.NewHandler(scheduleService)
	scheduleHandler.RegisterRoutes(r, authMiddleware, requireActiveUser, ownerWorkspaceMiddleware)

	financeRepository := finance.NewRepository(dbPool)
	financeService := finance.NewService(financeRepository)
	financeHandler := finance.NewHandler(financeService, auditService)
	financeHandler.RegisterRoutes(r, authMiddleware, requireActiveUser, ownerWorkspaceMiddleware)

	notificationsRepository := notifications.NewRepository(dbPool)
	notificationsService := notifications.NewService(notificationsRepository)
	notificationsHandler := notifications.NewHandler(notificationsService)
	notificationsGroup := r.Group("/notifications")
	notificationsGroup.Use(authMiddleware, requireActiveUser)
	notificationsHandler.RegisterRoutes(notificationsGroup)

	promosRepository := promos.NewRepository(dbPool)
	promosService := promos.NewService(promosRepository)
	promosHandler := promos.NewHandler(promosService)
	promosHandler.RegisterOwnerRoutes(r, authMiddleware, requireActiveUser, ownerWorkspaceMiddleware)
	promosHandler.RegisterCustomerRoutes(r, authMiddleware, requireActiveUser, middleware.RequireRole("CUSTOMER"))

	resolverAdapter := func(ctx context.Context, db platformfinance.CommercialTermQueryer, ownerProfileID string, effectiveAt time.Time) (*platformfinance.CommercialTerm, error) {
		resolver := platformfinance.NewCommercialTermResolver(db)
		return resolver.ResolveEffectiveTerm(ctx, ownerProfileID, effectiveAt)
	}
	calculator := platformfinance.CalculateBookingFees
	snapshotRepo := platformfinance.NewBookingFeeSnapshotRepository()

	snapshotOrchestrator, err := bookings.NewSnapshotOrchestrator(resolverAdapter, calculator, snapshotRepo)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize snapshot orchestrator: %w", err)
	}

	bookingsRepository := bookings.NewRepository(dbPool)
	bookingsService := bookings.NewService(bookingsRepository, cfg.BookingPaymentTTLMinutes, notificationsService, promosRepository, snapshotOrchestrator)
	bookingsHandler := bookings.NewHandler(bookingsService, auditService)
	bookingsHandler.RegisterRoutes(r, authMiddleware, requireActiveUser, middleware.RequireRole("CUSTOMER"))
	bookingsHandler.RegisterOwnerRoutes(r, authMiddleware, requireActiveUser, ownerWorkspaceMiddleware)

	blockedSlotRepository := blockedslots.NewRepository(dbPool)
	blockedSlotService := blockedslots.NewService(blockedSlotRepository)
	blockedSlotHandler := blockedslots.NewHandler(blockedSlotService)
	blockedSlotHandler.RegisterRoutes(r, authMiddleware, requireActiveUser, ownerWorkspaceMiddleware)

	mabarRepository := mabar.NewRepository(dbPool)
	mabarService := mabar.NewService(mabarRepository)
	mabarHandler := mabar.NewHandler(mabarService)
	mabarHandler.RegisterRoutes(r, authMiddleware, requireActiveUser, middleware.RequireRole("CUSTOMER"))

	refundsRepository := refunds.NewRepository(dbPool)
	refundsService := refunds.NewService(refundsRepository, notificationsService)
	refundsHandler := refunds.NewHandler(refundsService, auditService)
	refundsHandler.RegisterRoutes(r, authMiddleware, requireActiveUser, middleware.RequireRole("CUSTOMER"), ownerWorkspaceMiddleware)

	auditHandler := audit.NewHandler(auditService)
	auditHandler.RegisterRoutes(r, authMiddleware, requireActiveUser, ownerWorkspaceMiddleware)

	adminRepository := admin.NewRepository(dbPool)
	adminService := admin.NewService(adminRepository, auditService)
	adminHandler := admin.NewHandler(adminService)
	adminHandler.RegisterRoutes(r, authMiddleware, requireActiveUser)

	platformAuditRepository := audit.NewPlatformRepository()
	platformAuditService := audit.NewPlatformService(platformAuditRepository)

	buildServices := func() (platformfinance.Service, platformfinance.ExpenseService, platformfinance.JournalReadService, error) {
		return buildPlatformFinanceAdminServices(dbPool, platformAuditService)
	}

	if err := registerPlatformFinanceAdminModule(r, cfg.PlatformFinanceAdminEnabled, authMiddleware, requireActiveUser, middleware.RequireRole, buildServices); err != nil {
		return nil, nil, fmt.Errorf("failed to register platform finance admin module: %w", err)
	}
	if !cfg.PlatformFinanceAdminEnabled {
		log.Println("Platform Finance Admin routes are disabled via feature flag")
	}

	ctRepo := commercialterms.NewRepository(dbPool)
	ctService := commercialterms.NewService(ctRepo, dbPool, platformAuditService)
	commercialterms.RegisterRoutes(r, authMiddleware, requireActiveUser, middleware.RequireRole, ctService)

	analytics.RegisterRoutes(r, dbPool, authMiddleware, requireActiveUser, ownerWorkspaceMiddleware)

	workerCtx, workerCancel := context.WithCancel(context.Background())
	if startWorkers {
		go bookingsService.StartExpiryWorker(workerCtx, time.Duration(cfg.BookingExpirySweepIntervalSeconds)*time.Second)
		go bookingsService.StartAutoCompleteWorker(workerCtx, time.Duration(cfg.BookingAutoCompleteIntervalSeconds)*time.Second)
	}

	return r, workerCancel, nil
}

func buildPlatformFinanceAdminServices(dbPool *pgxpool.Pool, platformAuditService audit.PlatformService) (platformfinance.Service, platformfinance.ExpenseService, platformfinance.JournalReadService, error) {
	pfRepo := platformfinance.NewRepository(dbPool)
	pfService := platformfinance.NewService(pfRepo)

	expenseRepository := platformfinance.NewExpenseRepository(dbPool)
	journalService, err := platformfinance.NewJournalService(platformfinance.NewJournalRepository())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to initialize platform finance journal services: %w", err)
	}
	expenseService := platformfinance.NewExpenseService(expenseRepository, dbPool, platformAuditService, journalService)
	journalReadService, err := platformfinance.NewJournalReadService(platformfinance.NewJournalReadRepository(dbPool))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to initialize platform finance read services: %w", err)
	}

	return pfService, expenseService, journalReadService, nil
}

func registerPlatformFinanceAdminModule(
	r *gin.Engine,
	enabled bool,
	authMiddleware, requireActiveUser gin.HandlerFunc,
	requireRoleFactory func(...string) gin.HandlerFunc,
	buildServices func() (platformfinance.Service, platformfinance.ExpenseService, platformfinance.JournalReadService, error),
) error {
	if !enabled {
		return nil
	}

	pfService, expenseService, journalReadService, err := buildServices()
	if err != nil {
		return err
	}

	platformfinance.RegisterRoutes(r, authMiddleware, requireActiveUser, requireRoleFactory, pfService)
	platformfinance.RegisterExpenseRoutes(r, authMiddleware, requireActiveUser, requireRoleFactory, expenseService, journalReadService)
	return nil
}
