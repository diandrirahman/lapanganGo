package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"lapangango-api/internal/auth"
	"lapangango-api/internal/availability"
	"lapangango-api/internal/blockedslots"
	"lapangango-api/internal/bookings"
	"lapangango-api/internal/config"
	"lapangango-api/internal/courts"
	"lapangango-api/internal/database"
	"lapangango-api/internal/middleware"
	"lapangango-api/internal/owners"
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

	r := gin.Default()
	r.Use(middleware.CORS())

	tokenService := auth.NewTokenService(cfg.JWTSecret, cfg.JWTExpiresInHours)
	authRepository := auth.NewRepository(dbPool)
	authService := auth.NewService(authRepository, tokenService)
	authHandler := auth.NewHandler(authService)
	authMiddleware := middleware.Auth(tokenService)
	authHandler.RegisterRoutes(r, authMiddleware)

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

	bookingsRepository := bookings.NewRepository(dbPool)
	bookingsService := bookings.NewService(bookingsRepository)
	bookingsHandler := bookings.NewHandler(bookingsService)
	bookingsHandler.RegisterRoutes(r, authMiddleware, middleware.RequireRole("CUSTOMER"))

	blockedSlotRepository := blockedslots.NewRepository(dbPool)
	blockedSlotService := blockedslots.NewService(blockedSlotRepository)
	blockedSlotHandler := blockedslots.NewHandler(blockedSlotService)
	blockedSlotHandler.RegisterRoutes(r, authMiddleware, middleware.RequireRole("OWNER"))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"message": "LapanganGo API is running",
		})
	})

	r.GET("/db-health", func(c *gin.Context) {
		err := dbPool.Ping(ctx)
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

	ctxShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctxShutdown); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
}
