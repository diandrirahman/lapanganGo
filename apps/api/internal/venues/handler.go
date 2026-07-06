package venues

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"lapangango-api/internal/httputil"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router *gin.Engine, authMiddleware gin.HandlerFunc, ownerRoleMiddleware gin.HandlerFunc) {
	router.GET("/venues", h.GetPublicVenues)
	router.GET("/venues/:id", h.GetPublicVenue)
	router.GET("/sports", h.GetSports)
	router.GET("/facilities", h.GetFacilities)

	ownerGroup := router.Group("/owner", authMiddleware, ownerRoleMiddleware)
	ownerGroup.POST("/venues", h.CreateVenue)
	ownerGroup.GET("/venues", h.ListVenues)
	ownerGroup.GET("/venues/:id", h.GetVenue)
	ownerGroup.PUT("/venues/:id", h.UpdateVenue)
	ownerGroup.PATCH("/venues/:id/status", h.UpdateVenueStatus)
	ownerGroup.POST("/venues/:id/photos", h.AddVenuePhoto)
	ownerGroup.PUT("/venues/:id/photos/:photo_id", h.UpdateVenuePhoto)
	ownerGroup.DELETE("/venues/:id/photos/:photo_id", h.DeleteVenuePhoto)
}

func (h *Handler) GetPublicVenues(c *gin.Context) {
	req := ListPublicVenuesQuery{}
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid query parameters",
			"error":   err.Error(),
		})
		return
	}

	// Use httputil for robust parsing and limits
	pageParams := httputil.GetPaginationParams(c)
	req.Page = pageParams.Page
	req.Limit = pageParams.Limit

	venues, total, err := h.service.GetPublicVenues(c.Request.Context(), req)
	if err != nil {
		respondVenueError(c, err, "Failed to list public venues")
		return
	}

	c.JSON(http.StatusOK, httputil.NewPaginatedResponse(venues, total, req.Page, req.Limit))
}

func (h *Handler) GetPublicVenue(c *gin.Context) {
	venueID, ok := httputil.GetUUIDParam(c, "id", "Venue ID must be a valid UUID")
	if !ok {
		return
	}

	playDate := c.Query("play_date")
	if playDate != "" {
		if _, err := time.Parse("2006-01-02", playDate); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "Invalid play_date format. Use YYYY-MM-DD",
			})
			return
		}
	}

	venue, err := h.service.GetPublicVenue(c.Request.Context(), venueID, playDate)
	if err != nil {
		respondVenueError(c, err, "Failed to get public venue")
		return
	}

	c.JSON(http.StatusOK, venue)
}

func (h *Handler) GetSports(c *gin.Context) {
	sports, err := h.service.GetSports(c.Request.Context())
	if err != nil {
		respondVenueError(c, err, "Failed to get sports")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"sports": sports,
	})
}

func (h *Handler) GetFacilities(c *gin.Context) {
	facilities, err := h.service.GetFacilities(c.Request.Context())
	if err != nil {
		respondVenueError(c, err, "Failed to get facilities")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"facilities": facilities,
	})
}

func (h *Handler) CreateVenue(c *gin.Context) {
	userID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Unauthorized",
		})
		return
	}

	var req CreateVenueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid request payload",
			"error":   err.Error(),
		})
		return
	}

	venue, err := h.service.CreateVenue(c.Request.Context(), userID, req)
	if err != nil {
		respondVenueError(c, err, "Failed to create venue")
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Venue created successfully",
		"venue":   venue,
	})
}

func (h *Handler) ListVenues(c *gin.Context) {
	userID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Unauthorized",
		})
		return
	}

	venues, err := h.service.ListVenues(c.Request.Context(), userID)
	if err != nil {
		respondVenueError(c, err, "Failed to list venues")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"venues": venues,
	})
}

func (h *Handler) GetVenue(c *gin.Context) {
	userID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Unauthorized",
		})
		return
	}

	venueID, ok := httputil.GetUUIDParam(c, "id", "Venue ID must be a valid UUID")
	if !ok {
		return
	}

	venue, err := h.service.GetVenue(c.Request.Context(), userID, venueID)
	if err != nil {
		respondVenueError(c, err, "Failed to get venue")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"venue": venue,
	})
}

func (h *Handler) UpdateVenue(c *gin.Context) {
	userID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Unauthorized",
		})
		return
	}

	venueID, ok := httputil.GetUUIDParam(c, "id", "Venue ID must be a valid UUID")
	if !ok {
		return
	}

	var req UpdateVenueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid request payload",
			"error":   err.Error(),
		})
		return
	}

	venue, err := h.service.UpdateVenue(c.Request.Context(), userID, venueID, req)
	if err != nil {
		respondVenueError(c, err, "Failed to update venue")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Venue updated successfully",
		"venue":   venue,
	})
}

func (h *Handler) UpdateVenueStatus(c *gin.Context) {
	userID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Unauthorized",
		})
		return
	}

	venueID, ok := httputil.GetUUIDParam(c, "id", "Venue ID must be a valid UUID")
	if !ok {
		return
	}

	var req UpdateVenueStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid request payload",
			"error":   err.Error(),
		})
		return
	}

	venue, err := h.service.UpdateVenueStatus(c.Request.Context(), userID, venueID, req.Status)
	if err != nil {
		respondVenueError(c, err, "Failed to update venue status")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Venue status updated successfully",
		"venue":   venue,
	})
}

func respondVenueError(c *gin.Context, err error, fallbackMessage string) {
	switch {
	case errors.Is(err, ErrOwnerProfileNotFound):
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Owner profile is required before managing venues",
		})
	case errors.Is(err, ErrVenueNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"message": "Venue not found",
		})
	case errors.Is(err, ErrVenueAlreadyExists):
		c.JSON(http.StatusConflict, gin.H{
			"message": "Venue name already exists",
		})
	case errors.Is(err, ErrInvalidFacilities):
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "One or more facility IDs are invalid",
		})
	case errors.Is(err, ErrInvalidVenuePayload):
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Name, address, and city are required",
		})
	case errors.Is(err, ErrInvalidVenueStatus):
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid venue status",
		})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"message": fallbackMessage, "error": err.Error()})
	}
}

func (h *Handler) AddVenuePhoto(c *gin.Context) {
	userID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	venueID, ok := httputil.GetUUIDParam(c, "id", "Venue ID must be a valid UUID")
	if !ok {
		return
	}

	var req CreateVenuePhotoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid request body", "error": err.Error()})
		return
	}

	photo, err := h.service.AddVenuePhoto(c.Request.Context(), userID, venueID, req)
	if err != nil {
		respondVenueError(c, err, "Failed to add venue photo")
		return
	}

	c.JSON(http.StatusCreated, photo)
}

func (h *Handler) UpdateVenuePhoto(c *gin.Context) {
	userID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	venueID, ok := httputil.GetUUIDParam(c, "id", "Venue ID must be a valid UUID")
	if !ok {
		return
	}

	photoID, ok := httputil.GetUUIDParam(c, "photo_id", "Photo ID must be a valid UUID")
	if !ok {
		return
	}

	var req UpdateVenuePhotoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid request body", "error": err.Error()})
		return
	}

	photo, err := h.service.UpdateVenuePhoto(c.Request.Context(), userID, venueID, photoID, req)
	if err != nil {
		respondVenueError(c, err, "Failed to update venue photo")
		return
	}

	c.JSON(http.StatusOK, photo)
}

func (h *Handler) DeleteVenuePhoto(c *gin.Context) {
	userID, ok := httputil.GetAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized"})
		return
	}

	venueID, ok := httputil.GetUUIDParam(c, "id", "Venue ID must be a valid UUID")
	if !ok {
		return
	}

	photoID, ok := httputil.GetUUIDParam(c, "photo_id", "Photo ID must be a valid UUID")
	if !ok {
		return
	}

	err := h.service.DeleteVenuePhoto(c.Request.Context(), userID, venueID, photoID)
	if err != nil {
		respondVenueError(c, err, "Failed to delete venue photo")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Venue photo deleted successfully"})
}
