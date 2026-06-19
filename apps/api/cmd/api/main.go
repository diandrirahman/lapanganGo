package main

import (
	"context"
	"log"
	"net/http"

	"lapangango-api/internal/auth"
	"lapangango-api/internal/config"
	"lapangango-api/internal/database"
	"lapangango-api/internal/middleware"
	"lapangango-api/internal/owners"

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

	tokenService := auth.NewTokenService(cfg.JWTSecret, cfg.JWTExpiresInHours)
	authRepository := auth.NewRepository(dbPool)
	authService := auth.NewService(authRepository, tokenService)
	authHandler := auth.NewHandler(authService)
	authMiddleware := middleware.Auth(tokenService)
	authHandler.RegisterRoutes(r, authMiddleware)

	ownerRepository := owners.NewRepository(dbPool)
	ownerService := owners.NewService(ownerRepository)
	ownerHandler := owners.NewHandler(ownerService)
	ownerHandler.RegisterRoutes(r, authMiddleware, middleware.RequireRole("OWNER"))

	registerRoleTestRoutes(r, tokenService)

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

	log.Println("Server running on port", cfg.AppPort)

	if err := r.Run(":" + cfg.AppPort); err != nil {
		log.Fatal("Failed to run server:", err)
	}
}

func registerRoleTestRoutes(r *gin.Engine, tokenService *auth.TokenService) {
	authMiddleware := middleware.Auth(tokenService)

	r.GET("/customer/profile-test", authMiddleware, middleware.RequireRole("CUSTOMER"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "Customer access granted",
		})
	})

	r.GET("/owner/dashboard-test", authMiddleware, middleware.RequireRole("OWNER"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "Owner access granted",
		})
	})

	r.GET("/admin/dashboard-test", authMiddleware, middleware.RequireRole("SUPER_ADMIN"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "Super admin access granted",
		})
	})
}
