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

func main() {
	r := gin.Default()

	// API v1 group
	v1 := r.Group("/api/v1")
	{
		v1.GET("/health", healthCheck)
	}

	// Swagger documentation
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	if err := r.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
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
