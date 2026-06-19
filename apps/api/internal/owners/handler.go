package owners

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router *gin.Engine, authMiddleware gin.HandlerFunc, ownerRoleMiddleware gin.HandlerFunc) {
	ownerGroup := router.Group("/owner", authMiddleware, ownerRoleMiddleware)
	ownerGroup.POST("/profile", h.CreateProfile)
	ownerGroup.GET("/profile", h.GetProfile)
	ownerGroup.PUT("/profile", h.UpdateProfile)
}

func (h *Handler) CreateProfile(c *gin.Context) {
	userID, ok := getAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Unauthorized",
		})
		return
	}

	var req CreateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid request payload",
			"error":   err.Error(),
		})
		return
	}

	profile, err := h.service.CreateProfile(c.Request.Context(), userID, req)
	if err != nil {
		if errors.Is(err, ErrProfileAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{
				"message": "Owner profile already exists",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "Failed to create owner profile",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Owner profile created successfully",
		"profile": profile,
	})
}

func (h *Handler) GetProfile(c *gin.Context) {
	userID, ok := getAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Unauthorized",
		})
		return
	}

	profile, err := h.service.GetProfile(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrProfileNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"message": "Owner profile not found",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "Failed to get owner profile",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"profile": profile,
	})
}

func (h *Handler) UpdateProfile(c *gin.Context) {
	userID, ok := getAuthenticatedUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Unauthorized",
		})
		return
	}

	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid request payload",
			"error":   err.Error(),
		})
		return
	}

	profile, err := h.service.UpdateProfile(c.Request.Context(), userID, req)
	if err != nil {
		if errors.Is(err, ErrProfileNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"message": "Owner profile not found",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "Failed to update owner profile",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Owner profile updated successfully",
		"profile": profile,
	})
}

func getAuthenticatedUserID(c *gin.Context) (string, bool) {
	userIDValue, exists := c.Get("auth_user_id")
	if !exists {
		return "", false
	}

	userID, ok := userIDValue.(string)
	return userID, ok && userID != ""
}
