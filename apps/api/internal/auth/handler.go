package auth

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

func (h *Handler) RegisterRoutes(router *gin.Engine, authMiddleware gin.HandlerFunc) {
	authGroup := router.Group("/auth")
	authGroup.POST("/register", h.Register)
	authGroup.POST("/login", h.Login)
	authGroup.GET("/me", authMiddleware, h.Me)
}

func (h *Handler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid request payload",
			"error":   err.Error(),
		})
		return
	}

	user, err := h.service.Register(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, ErrEmailAlreadyUsed):
			c.JSON(http.StatusConflict, gin.H{
				"message": "Email already used",
			})
			return
		case errors.Is(err, ErrPhoneAlreadyUsed):
			c.JSON(http.StatusConflict, gin.H{
				"message": "Phone already used",
			})
			return
		case errors.Is(err, ErrUnsupportedRegistrationRole):
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "Public registration only supports customer accounts",
			})
			return
		default:
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": "Failed to register user",
			})
			return
		}
	}

	c.JSON(http.StatusCreated, AuthResponse{
		Message: "User registered successfully",
		User:    user,
	})
}

func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid request payload",
			"error":   err.Error(),
		})
		return
	}

	loginResponse, err := h.service.Login(c.Request.Context(), req)
	if err != nil {
		if errors.Is(err, ErrInvalidCredential) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"message": "Invalid email or password",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "Failed to login",
		})
		return
	}

	c.JSON(http.StatusOK, loginResponse)
}

func (h *Handler) Me(c *gin.Context) {
	emailValue, exists := c.Get("auth_email")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Unauthorized",
		})
		return
	}

	email, ok := emailValue.(string)
	if !ok || email == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Unauthorized",
		})
		return
	}

	user, err := h.service.GetUserByEmail(c.Request.Context(), email)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Unauthorized",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": user,
	})
}
