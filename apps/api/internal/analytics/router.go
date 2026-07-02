package analytics

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func RegisterRoutes(r *gin.Engine, db *pgxpool.Pool, authMiddleware gin.HandlerFunc, roleMiddleware gin.HandlerFunc) {
	repo := NewRepository(db)
	svc := NewService(repo)
	handler := NewHandler(svc)

	ownerAnalytics := r.Group("/owner/analytics")
	ownerAnalytics.Use(authMiddleware, roleMiddleware)
	{
		ownerAnalytics.GET("/bookings", handler.GetBookingsTrend)
		ownerAnalytics.GET("/revenue", handler.GetRevenueTrend)
		ownerAnalytics.GET("/status", handler.GetStatusBreakdown)
		ownerAnalytics.GET("/expenses", handler.GetExpensesBreakdown)
	}
}
