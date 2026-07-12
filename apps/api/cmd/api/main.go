package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
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
	"lapangango-api/internal/database"
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
)

func main() {
	cfg := config.Load()

	ctx := context.Background()

	dbPool, err := database.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal("Failed to connect to PostgreSQL:", err)
	}
	defer dbPool.Close()

	log.Println("Starting database migrations...")
	if err := database.RunMigrations(cfg.DatabaseURL); err != nil {
		log.Fatal("Migration failed: ", err)
	} else {
		log.Println("Database migrations completed successfully.")
	}

	if err := database.EnsureBookingSchema(ctx, dbPool); err != nil {
		log.Fatal("Failed to ensure booking schema:", err)
	}

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

	// Increased rate limit for local demo/QA to prevent lockouts
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
	// Notifications routes apply to all authenticated users
	notificationsGroup := r.Group("/notifications")
	notificationsGroup.Use(authMiddleware, requireActiveUser)
	notificationsHandler.RegisterRoutes(notificationsGroup)

	promosRepository := promos.NewRepository(dbPool)
	promosService := promos.NewService(promosRepository)
	promosHandler := promos.NewHandler(promosService)
	promosHandler.RegisterOwnerRoutes(r, authMiddleware, requireActiveUser, ownerWorkspaceMiddleware)
	promosHandler.RegisterCustomerRoutes(r, authMiddleware, requireActiveUser, middleware.RequireRole("CUSTOMER"))

	bookingsRepository := bookings.NewRepository(dbPool)
	bookingsService := bookings.NewService(bookingsRepository, cfg.BookingPaymentTTLMinutes, notificationsService, promosRepository)
	bookingsHandler := bookings.NewHandler(bookingsService, auditService)
	bookingsHandler.RegisterRoutes(r, authMiddleware, requireActiveUser, middleware.RequireRole("CUSTOMER"))
	bookingsHandler.RegisterOwnerRoutes(r, authMiddleware, requireActiveUser, ownerWorkspaceMiddleware)

	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	go bookingsService.StartExpiryWorker(workerCtx, time.Duration(cfg.BookingExpirySweepIntervalSeconds)*time.Second)
	go bookingsService.StartAutoCompleteWorker(workerCtx, time.Duration(cfg.BookingAutoCompleteIntervalSeconds)*time.Second)

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

	pfRepo := platformfinance.NewRepository(dbPool)
	pfService := platformfinance.NewService(pfRepo)
	platformfinance.RegisterRoutes(r, authMiddleware, requireActiveUser, middleware.RequireRole, pfService)

	platformAuditRepository := audit.NewPlatformRepository()
	platformAuditService := audit.NewPlatformService(platformAuditRepository)

	ctRepo := commercialterms.NewRepository(dbPool)
	ctService := commercialterms.NewService(ctRepo, dbPool, platformAuditService)
	commercialterms.RegisterRoutes(r, authMiddleware, requireActiveUser, middleware.RequireRole, ctService)

	analytics.RegisterRoutes(r, dbPool, authMiddleware, requireActiveUser, ownerWorkspaceMiddleware)
	srv := &http.Server{
		Addr:    ":" + cfg.AppPort,
		Handler: r,
	}

	go func() {
		log.Println("Server running on port", cfg.AppPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to run server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")
	workerCancel()

	ctxShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctxShutdown); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
}
