package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	_ "github.com/ethbucharest25/go-web3-superbase/backend/docs"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
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

type MetabaseDashboardRequest struct {
	TenantId string `json:"tenant_id" binding:"required"`
}

type AddContractRequest struct {
	TenantId        string `json:"tenant_id" binding:"required"`
	ContractAddress string `json:"contract_address" binding:"required"`
	ContractABI     string `json:"contract_abi" binding:"required"`
	StartBlock      uint64 `json:"start_block" binding:"required"`
}

type TenantServiceRequest struct {
	TenantId string `json:"tenant_id" binding:"required"`
}

type TenantService struct {
	ID               string          `json:"id"`
	TenantID         string          `json:"tenant_id"`
	ServiceType      string          `json:"service_type"`
	ContainerID      *string         `json:"container_id"`
	TargetIdentifier string          `json:"target_identifier"`
	LastProcessedID  string          `json:"last_processed_id"`
	Status           string          `json:"status"`
	Config           json.RawMessage `json:"config"`
	Metadata         json.RawMessage `json:"metadata"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

func main() {
	r := gin.Default()

	// Configure CORS - Allow all for hackathon
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Length", "Content-Type", "Accept", "Authorization", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length", "Content-Type"},
		AllowCredentials: false, // Must be false when AllowOrigins is *
		MaxAge:           12 * time.Hour,
	}))

	// Connect to database
	db, err := connectToDatabase()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Initialize auth service
	authService := NewAuthService()

	// API v1 group
	v1 := r.Group("/api/v1")
	{
		v1.GET("/health", healthCheck)
		v1.POST("/register", registerUser(authService))
		v1.POST("/user/info", getUserWithTenant(authService))
		v1.POST("/metabase/dashboard", getMetabaseDashboard)

		// Service runner routes
		v1.POST("/contracts", addContract(db))
		v1.POST("/services", getTenantServices(db))
		v1.POST("/services/start", startService(db))
		v1.POST("/services/stop", stopService(db))
	}

	// Swagger documentation
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	if err := r.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func connectToDatabase() (*pgxpool.Pool, error) {
	// Get database connection parameters from environment
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbName := os.Getenv("DB_NAME")

	// Construct connection string
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=require",
		dbUser, dbPassword, dbHost, dbPort, dbName)

	// Initialize database connection with context
	ctx := context.Background()

	// Create connection pool
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %v", err)
	}

	// Test database connection
	var result int
	err = pool.QueryRow(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to verify database connection: %v", err)
	}

	return pool, nil
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

// @Summary     Get Metabase dashboard URL with JWT token
// @Description Generates a signed JWT for embedding Metabase dashboard with tenant_id parameter
// @Tags        dashboard
// @Accept      json
// @Produce     json
// @Param       request body MetabaseDashboardRequest true "Tenant ID"
// @Success     200 {object} map[string]string
// @Failure     400 {object} map[string]string
// @Failure     500 {object} map[string]string
// @Router      /metabase/dashboard [post]
func getMetabaseDashboard(c *gin.Context) {
	var req MetabaseDashboardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get environment variables
	metabaseSecretKey := os.Getenv("METABASE_SECRET_KEY")
	metabaseSiteURL := os.Getenv("METABASE_SITE_URL")
	dashboardID := os.Getenv("METABASE_DASHBOARD_ID")

	if metabaseSecretKey == "" || metabaseSiteURL == "" || dashboardID == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Metabase configuration is missing"})
		return
	}

	// Convert dashboard ID to an integer if possible
	dashboardIDInt, err := strconv.Atoi(dashboardID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid dashboard ID format"})
		return
	}

	// Create JWT payload for static embedding
	claims := jwt.MapClaims{
		"resource": map[string]interface{}{
			"dashboard": dashboardIDInt,
		},
		"params": map[string]interface{}{
			"tenant_id": []string{req.TenantId},
		},
		"exp": time.Now().Add(time.Hour * 10).Unix(), // 10 hour expiration
	}

	// Create JWT token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign token with secret key
	tokenString, err := token.SignedString([]byte(metabaseSecretKey))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate dashboard token"})
		return
	}

	// Create the final iframe URL with more customization options
	signedURL := metabaseSiteURL + "/embed/dashboard/" + tokenString + "#bordered=true&titled=false&theme=light&hide_parameters=true"

	c.JSON(http.StatusOK, gin.H{
		"signed_url": signedURL,
		"debug_info": map[string]interface{}{
			"metabase_url": metabaseSiteURL,
			"dashboard_id": dashboardIDInt,
			"tenant_id":    req.TenantId,
			"token_exp":    time.Now().Add(time.Hour * 10).Unix(),
		},
	})
}

// @Summary     Add a new smart contract for indexing
// @Description Register a smart contract with ABI to be indexed for a tenant
// @Tags        contracts
// @Accept      json
// @Produce     json
// @Param       request body AddContractRequest true "Contract details"
// @Success     200 {object} map[string]interface{}
// @Failure     400 {object} map[string]string
// @Failure     500 {object} map[string]string
// @Router      /contracts [post]
func addContract(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req AddContractRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Validate tenant ID
		_, err := uuid.Parse(req.TenantId)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID format"})
			return
		}

		// Validate contract address format (simple check for now)
		if !strings.HasPrefix(req.ContractAddress, "0x") || len(req.ContractAddress) != 42 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid contract address format"})
			return
		}

		// Validate start block
		if req.StartBlock == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Start block must be greater than 0"})
			return
		}

		// Add contract to tenant_services table
		ctx := c.Request.Context()
		var serviceID int
		err = db.QueryRow(ctx, `
			INSERT INTO tenant_services
			(tenant_id, service_type, target_identifier, last_processed_id, status, config, metadata)
			VALUES ($1::uuid, 'evm-indexer', $2, $3, 'created', $4, $5)
			ON CONFLICT (tenant_id, service_type, target_identifier)
			DO UPDATE SET
				status = 'created',
				updated_at = CURRENT_TIMESTAMP,
				config = $4,
				last_processed_id = $3
			RETURNING id
		`, req.TenantId, req.ContractAddress,
			strconv.FormatUint(req.StartBlock, 10),
			json.RawMessage(fmt.Sprintf(`{"contract_abi": %q}`, req.ContractABI)),
			json.RawMessage(fmt.Sprintf(`{"start_block": %d}`, req.StartBlock))).Scan(&serviceID)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to register contract: %v", err)})
			return
		}

		// Start the indexer automatically
		go startEvmIndexer(db, req.TenantId, req.ContractAddress, req.ContractABI, req.StartBlock)

		c.JSON(http.StatusOK, gin.H{
			"message":    "Contract registered for indexing",
			"service_id": serviceID,
		})
	}
}

// @Summary     Get tenant services
// @Description Retrieve all services for a tenant
// @Tags        services
// @Accept      json
// @Produce     json
// @Param       request body TenantServiceRequest true "Tenant ID"
// @Success     200 {array} TenantService
// @Failure     400 {object} map[string]string
// @Failure     500 {object} map[string]string
// @Router      /services [post]
func getTenantServices(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req TenantServiceRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Validate tenant ID
		_, err := uuid.Parse(req.TenantId)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tenant ID format"})
			return
		}

		// Query services for this tenant
		ctx := c.Request.Context()
		rows, err := db.Query(ctx, `
			SELECT id, tenant_id, service_type, container_id, target_identifier,
				   COALESCE(last_processed_id, '') as last_processed_id,
				   status, config, COALESCE(metadata, '{}') as metadata,
				   created_at, updated_at
			FROM tenant_services
			WHERE tenant_id = $1::uuid
			ORDER BY created_at DESC
		`, req.TenantId)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch services: %v", err)})
			return
		}
		defer rows.Close()

		services := []TenantService{}
		for rows.Next() {
			var s TenantService
			var configBytes []byte
			var metadataBytes []byte

			if err := rows.Scan(
				&s.ID, &s.TenantID, &s.ServiceType, &s.ContainerID,
				&s.TargetIdentifier, &s.LastProcessedID, &s.Status, &configBytes,
				&metadataBytes, &s.CreatedAt, &s.UpdatedAt); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to process row: %v", err)})
				return
			}

			s.Config = json.RawMessage(configBytes)
			s.Metadata = json.RawMessage(metadataBytes)
			services = append(services, s)
		}

		if err := rows.Err(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error during row iteration: %v", err)})
			return
		}

		c.JSON(http.StatusOK, services)
	}
}

// @Summary     Start a service
// @Description Start a service for a tenant
// @Tags        services
// @Accept      json
// @Produce     json
// @Param       request body TenantServiceRequest true "Tenant ID"
// @Success     200 {object} map[string]string
// @Failure     400 {object} map[string]string
// @Failure     500 {object} map[string]string
// @Router      /services/start [post]
func startService(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req TenantServiceRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Get tenant services that are not running
		ctx := c.Request.Context()
		rows, err := db.Query(ctx, `
			SELECT id, service_type, target_identifier, config, COALESCE(last_processed_id, '0')
			FROM tenant_services
			WHERE tenant_id = $1::uuid
			AND (status = 'created' OR status = 'stopped' OR status = 'failed')
		`, req.TenantId)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch services: %v", err)})
			return
		}
		defer rows.Close()

		// Start each service
		startedCount := 0
		for rows.Next() {
			var id int
			var serviceType string
			var targetIdentifier string
			var configBytes []byte
			var lastProcessedID string

			if err := rows.Scan(&id, &serviceType, &targetIdentifier, &configBytes, &lastProcessedID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to process row: %v", err)})
				return
			}

			// Handle different service types
			switch serviceType {
			case "evm-indexer":
				// Extract ABI from config
				var config map[string]interface{}
				if err := json.Unmarshal(configBytes, &config); err != nil {
					log.Printf("Failed to unmarshal config for service %d: %v", id, err)
					continue
				}

				contractABI, ok := config["contract_abi"].(string)
				if !ok {
					log.Printf("Missing contract_abi in config for service %d", id)
					continue
				}

				// Parse the last processed ID as a start block
				startBlock, err := strconv.ParseUint(lastProcessedID, 10, 64)
				if err != nil || startBlock == 0 {
					log.Printf("Invalid last_processed_id for service %d: %s", id, lastProcessedID)
					continue
				}

				// Start the EVM indexer
				go startEvmIndexer(db, req.TenantId, targetIdentifier, contractABI, startBlock)
				startedCount++

			// Add other service types here as needed
			default:
				log.Printf("Unknown service type: %s for service %d", serviceType, id)
				continue
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"message": fmt.Sprintf("Started %d services", startedCount),
		})
	}
}

// @Summary     Stop a service
// @Description Stop a service for a tenant
// @Tags        services
// @Accept      json
// @Produce     json
// @Param       request body TenantServiceRequest true "Tenant ID"
// @Success     200 {object} map[string]string
// @Failure     400 {object} map[string]string
// @Failure     500 {object} map[string]string
// @Router      /services/stop [post]
func stopService(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req TenantServiceRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Get tenant services that are running
		ctx := c.Request.Context()
		rows, err := db.Query(ctx, `
			SELECT id, container_id
			FROM tenant_services
			WHERE tenant_id = $1::uuid
			AND status = 'running'
			AND container_id IS NOT NULL
		`, req.TenantId)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch services: %v", err)})
			return
		}
		defer rows.Close()

		// Stop each service
		stoppedCount := 0
		for rows.Next() {
			var id int
			var containerID string

			if err := rows.Scan(&id, &containerID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to process row: %v", err)})
				return
			}

			// Stop the container
			stopCmd := exec.Command("docker", "stop", containerID)
			if err := stopCmd.Run(); err != nil {
				log.Printf("Failed to stop container %s: %v", containerID, err)
				continue
			}

			// Update service status
			_, err = db.Exec(ctx, `
				UPDATE tenant_services
				SET status = 'stopped',
					updated_at = CURRENT_TIMESTAMP,
					container_id = NULL
				WHERE id = $1
			`, id)

			if err != nil {
				log.Printf("Failed to update service %d status: %v", id, err)
				continue
			}

			stoppedCount++
		}

		c.JSON(http.StatusOK, gin.H{
			"message": fmt.Sprintf("Stopped %d services", stoppedCount),
		})
	}
}

// Start the EVM indexer for a tenant
func startEvmIndexer(db *pgxpool.Pool, tenantID string, contractAddress string, contractABI string, startBlock uint64) {
	log.Printf("Starting EVM indexer for tenant %s, contract %s from block %d", tenantID, contractAddress, startBlock)

	// Create a unique container name
	containerName := fmt.Sprintf("evm-indexer-%s-%s", tenantID[:8], contractAddress[2:10])

	// Prepare the docker run command
	cmd := exec.Command(
		"docker", "run",
		"-d", "--name", containerName,
		"--network", "go-web3-superbase_default", // Connect to the project's docker network
		"-e", fmt.Sprintf("DB_USER=%s", os.Getenv("DB_USER")),
		"-e", fmt.Sprintf("DB_PASSWORD=%s", os.Getenv("DB_PASSWORD")),
		"-e", fmt.Sprintf("DB_HOST=%s", os.Getenv("DB_HOST")),
		"-e", fmt.Sprintf("DB_PORT=%s", os.Getenv("DB_PORT")),
		"-e", fmt.Sprintf("DB_NAME=%s", os.Getenv("DB_NAME")),
		"-e", fmt.Sprintf("RPC_URL=%s", os.Getenv("RPC_URL")),
		"-e", fmt.Sprintf("INFURA_API_KEY=%s", os.Getenv("INFURA_API_KEY")),
		"-e", fmt.Sprintf("INFURA_API_SECRET=%s", os.Getenv("INFURA_API_SECRET")),
		"evm-indexer:latest", // The Docker image name
		"-tenant", tenantID,
		"-contract", contractAddress,
		"-abi", contractABI,
		"-startBlock", strconv.FormatUint(startBlock, 10),
	)

	// Run the command
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Failed to start indexer container: %v, output: %s", err, string(output))

		// Update service status to failed
		ctx := context.Background()
		_, dbErr := db.Exec(ctx, `
			UPDATE tenant_services
			SET status = 'failed',
				updated_at = CURRENT_TIMESTAMP,
				metadata = jsonb_set(COALESCE(metadata, '{}'::jsonb), '{error}', $1::jsonb)
			WHERE tenant_id = $2::uuid
			AND service_type = 'evm-indexer'
			AND target_identifier = $3
		`, json.RawMessage(fmt.Sprintf(`"%s"`, err.Error())), tenantID, contractAddress)

		if dbErr != nil {
			log.Printf("Failed to update service status: %v", dbErr)
		}

		return
	}

	// Extract container ID from docker run output
	containerID := strings.TrimSpace(string(output))

	// Update service record with container ID
	ctx := context.Background()
	_, err = db.Exec(ctx, `
		UPDATE tenant_services
		SET status = 'running',
			updated_at = CURRENT_TIMESTAMP,
			container_id = $1,
			metadata = jsonb_set(COALESCE(metadata, '{}'::jsonb), '{started_at}', $2::jsonb)
		WHERE tenant_id = $3::uuid
		AND service_type = 'evm-indexer'
		AND target_identifier = $4
	`, containerID, json.RawMessage(fmt.Sprintf(`"%s"`, time.Now().Format(time.RFC3339))), tenantID, contractAddress)

	if err != nil {
		log.Printf("Failed to update service with container ID: %v", err)
	}

	log.Printf("Started indexer container %s for tenant %s", containerID, tenantID)
}
