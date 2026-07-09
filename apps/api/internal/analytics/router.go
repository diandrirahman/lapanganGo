package analytics

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"lapangango-api/internal/middleware"
)

func RegisterRoutes(r *gin.Engine, db *pgxpool.Pool, authMiddleware gin.HandlerFunc, ownerWorkspaceMiddleware gin.HandlerFunc) {
	repo := NewRepository(db)
	svc := NewService(repo)
	handler := NewHandler(svc)

	ownerAnalytics := r.Group("/owner/analytics")
	ownerAnalytics.Use(authMiddleware, ownerWorkspaceMiddleware)
	{
		ownerAnalytics.GET("/bookings", middleware.RequireOwnerPermission("ANALYTICS_READ"), handler.GetBookingsTrend)
		ownerAnalytics.GET("/revenue", middleware.RequireOwnerPermission("ANALYTICS_READ"), handler.GetRevenueTrend)
		ownerAnalytics.GET("/status", middleware.RequireOwnerPermission("ANALYTICS_READ"), handler.GetStatusBreakdown)
		ownerAnalytics.GET("/expenses", middleware.RequireOwnerPermission("ANALYTICS_READ"), handler.GetExpensesBreakdown)
	}
}
