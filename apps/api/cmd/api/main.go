package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"lapangango-api/internal/analytics"
	"lapangango-api/internal/auth"
	"lapangango-api/internal/availability"
	"lapangango-api/internal/blockedslots"
	"lapangango-api/internal/bookings"
	"lapangango-api/internal/config"
	"lapangango-api/internal/courts"
	"lapangango-api/internal/database"
	"lapangango-api/internal/finance"
	"lapangango-api/internal/mabar"
	"lapangango-api/internal/middleware"
	"lapangango-api/internal/notifications"
	"lapangango-api/internal/owners"
	"lapangango-api/internal/promos"
	"lapangango-api/internal/refunds"
	"lapangango-api/internal/schedules"
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

	// Increased rate limit for local demo/QA to prevent lockouts
	authRateLimiter := middleware.NewRateLimiter(cfg.RedisURL, "auth", cfg.AuthRateLimitPerMinute, time.Minute)
	authHandler.RegisterRoutes(r, authMiddleware, authRateLimiter)

	availabilityRepository := availability.NewRepository(dbPool)
	availabilityService := availability.NewService(availabilityRepository)
	availabilityHandler := availability.NewHandler(availabilityService)
	availabilityHandler.RegisterRoutes(r)

	ownerRepository := owners.NewRepository(dbPool)
	ownerService := owners.NewService(ownerRepository)
	ownerHandler := owners.NewHandler(ownerService)
	ownerHandler.RegisterRoutes(r, authMiddleware, middleware.RequireRole("OWNER"))

	venueRepository := venues.NewRepository(dbPool)
	venueService := venues.NewService(venueRepository)
	venueHandler := venues.NewHandler(venueService)
	venueHandler.RegisterRoutes(r, authMiddleware, middleware.RequireRole("OWNER"))

	courtRepository := courts.NewRepository(dbPool)
	courtService := courts.NewService(courtRepository)
	courtHandler := courts.NewHandler(courtService)
	courtHandler.RegisterRoutes(r, authMiddleware, middleware.RequireRole("OWNER"))

	scheduleRepository := schedules.NewRepository(dbPool)
	scheduleService := schedules.NewService(scheduleRepository)
	scheduleHandler := schedules.NewHandler(scheduleService)
	scheduleHandler.RegisterRoutes(r, authMiddleware, middleware.RequireRole("OWNER"))

	financeRepository := finance.NewRepository(dbPool)
	financeService := finance.NewService(financeRepository)
	financeHandler := finance.NewHandler(financeService)
	financeHandler.RegisterRoutes(r, authMiddleware, middleware.RequireRole("OWNER"))

	notificationsRepository := notifications.NewRepository(dbPool)
	notificationsService := notifications.NewService(notificationsRepository)
	notificationsHandler := notifications.NewHandler(notificationsService)
	// Notifications routes apply to all authenticated users
	notificationsGroup := r.Group("/notifications")
	notificationsGroup.Use(authMiddleware)
	notificationsHandler.RegisterRoutes(notificationsGroup)

	promosRepository := promos.NewRepository(dbPool)
	promosService := promos.NewService(promosRepository)
	promosHandler := promos.NewHandler(promosService)
	promosHandler.RegisterOwnerRoutes(r, authMiddleware, middleware.RequireRole("OWNER"))
	promosHandler.RegisterCustomerRoutes(r, authMiddleware, middleware.RequireRole("CUSTOMER"))

	bookingsRepository := bookings.NewRepository(dbPool)
	bookingsService := bookings.NewService(bookingsRepository, cfg.BookingPaymentTTLMinutes, notificationsService, promosRepository)
	bookingsHandler := bookings.NewHandler(bookingsService)
	bookingsHandler.RegisterRoutes(r, authMiddleware, middleware.RequireRole("CUSTOMER"))
	bookingsHandler.RegisterOwnerRoutes(r, authMiddleware, middleware.RequireRole("OWNER"))

	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	go bookingsService.StartExpiryWorker(workerCtx, time.Duration(cfg.BookingExpirySweepIntervalSeconds)*time.Second)
	go bookingsService.StartAutoCompleteWorker(workerCtx, time.Duration(cfg.BookingAutoCompleteIntervalSeconds)*time.Second)

	blockedSlotRepository := blockedslots.NewRepository(dbPool)
	blockedSlotService := blockedslots.NewService(blockedSlotRepository)
	blockedSlotHandler := blockedslots.NewHandler(blockedSlotService)
	blockedSlotHandler.RegisterRoutes(r, authMiddleware, middleware.RequireRole("OWNER"))

	mabarRepository := mabar.NewRepository(dbPool)
	mabarService := mabar.NewService(mabarRepository)
	mabarHandler := mabar.NewHandler(mabarService)
	mabarHandler.RegisterRoutes(r, authMiddleware, middleware.RequireRole("CUSTOMER"))

	refundsRepository := refunds.NewRepository(dbPool)
	refundsService := refunds.NewService(refundsRepository, notificationsService)
	refundsHandler := refunds.NewHandler(refundsService)
	refundsHandler.RegisterRoutes(r, authMiddleware, middleware.RequireRole("CUSTOMER"), middleware.RequireRole("OWNER"))

	analytics.RegisterRoutes(r, dbPool, authMiddleware, middleware.RequireRole("OWNER"))
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
