package main

import (
	"log"
	"net/http"

	_ "github.com/ethbucharest25/go-web3-superbase/backend/docs"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// @title           Go Web3 Superbase Backend API
// @version         1.0
// @description     This is the backend API for the Go Web3 Superbase project.
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.url    http://www.swagger.io/support
// @contact.email  support@swagger.io

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      localhost:8080
// @BasePath  /api/v1

type RegisterRequest struct {
	Email      string `json:"email" binding:"required,email"`
	Password   string `json:"password" binding:"required,min=8"`
	TenantName string `json:"tenant_name" binding:"required"`
}

type UserInfoRequest struct {
	UserId string `json:"user_id" binding:"required"`
}

func main() {
	r := gin.Default()

	// Initialize auth service
	authService := NewAuthService()

	// API v1 group
	v1 := r.Group("/api/v1")
	{
		v1.GET("/health", healthCheck)
		v1.POST("/register", registerUser(authService))
		v1.POST("/user/info", getUserWithTenant(authService))
	}

	// Swagger documentation
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	if err := r.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// @Summary     Register a new user
// @Description Register a new user with email, password and tenant name
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       request body RegisterRequest true "Registration details"
// @Success     200 {object} map[string]string
// @Failure     400 {object} map[string]string
// @Failure     500 {object} map[string]string
// @Router      /register [post]
func registerUser(authService *AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req RegisterRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		userId, err := authService.RegisterUser(req.Email, req.Password, req.TenantName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "User registered successfully",
			"user_id": userId,
		})
	}
}

// @Summary     Health check endpoint
// @Description Check if the API is running
// @Tags        health
// @Accept      json
// @Produce     json
// @Success     200 {object} map[string]string
// @Router      /health [get]
func healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
}

// @Summary     Get user with tenant information
// @Description Retrieve user information along with their tenant and role
// @Tags        user
// @Accept      json
// @Produce     json
// @Param       request body UserInfoRequest true "User ID"
// @Success     200 {object} map[string]interface{}
// @Failure     400 {object} map[string]string
// @Failure     404 {object} map[string]string
// @Failure     500 {object} map[string]string
// @Router      /user/info [post]
func getUserWithTenant(authService *AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req UserInfoRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		userInfo, err := authService.GetUserWithTenantInfo(req.UserId)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if userInfo == nil || len(userInfo) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found or has no tenant information"})
			return
		}

		c.JSON(http.StatusOK, userInfo)
	}
}
