# Data Processor Guide for Web3 Insights Platform

This guide explains how to build a data processor for the Web3 Insights Platform. The platform uses a microservices architecture where each processor is a containerized service that processes specific data types.

## Existing Processors

Currently, the platform includes:

1. **EVM Indexer** - A data processor that indexes events from Ethereum and EVM-compatible blockchain contracts.

## Processors That Could Be Added

Based on the frontend configuration, these processors could be implemented:

1. **Luma Attendees Processor** - For processing CSV data from Luma event attendance.
2. **GitHub Analysis Processor** - For analyzing GitHub data with search keywords.

## Data Processor Architecture

A data processor in this system consists of:

1. A Go-based service in `packages/`
2. A Docker image definition
3. An entry in the `docker-compose.yml` for the image builder
4. Database schema for storing processor configuration and data
5. Frontend components for configuration (optional)

## How to Build a New Data Processor

### 1. Create the Processor Structure

```bash
mkdir -p packages/your-processor-name
cd packages/your-processor-name
```

### 2. Initialize Go Module

```bash
go mod init your-processor-name
```

### 3. Create Dockerfile

Create a `Dockerfile` in your processor directory:

```dockerfile
FROM golang:1.23-bookworm AS builder

WORKDIR /app

# Install required dependencies
RUN apt-get update && apt-get install -y git

# Copy go module files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY main.go ./

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o processor-name .

# Create a minimal production image
FROM debian:bookworm-slim

# Add ca-certificates for secure connections
RUN apt-get update && apt-get install -y ca-certificates tzdata && rm -rf /var/lib/apt/lists/*

WORKDIR /root/

# Copy the binary from the builder stage
COPY --from=builder /app/processor-name .

# Make the binary executable
RUN chmod +x ./processor-name

# Create a non-root user to run the application
RUN useradd -ms /bin/bash appuser
USER appuser

# Command to run the executable
ENTRYPOINT ["./processor-name"]
```

### 4. Create Environment Configuration

Create a `.env.example` file in your processor directory:

```
# Database Configuration
DB_USER=your_db_user
DB_PASSWORD=your_db_password
DB_HOST=your_db_host
DB_PORT=your_db_port
DB_NAME=your_db_name

# Processor-specific configuration
PROCESSOR_PARAM_1=value1
PROCESSOR_PARAM_2=value2
```

### 5. Create Database Schema

Create a SQL file in `packages/backend/db/` for your processor's tables:

```sql
-- Create your processor's tables
CREATE TABLE IF NOT EXISTS your_processor_table (
    id SERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL,
    processed_item_id VARCHAR(100) NOT NULL,
    processed_data JSONB NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Add indexes
CREATE INDEX IF NOT EXISTS idx_your_processor_tenant_id ON your_processor_table(tenant_id);

-- Create a unique constraint if needed
ALTER TABLE your_processor_table ADD CONSTRAINT unique_tenant_item 
    UNIQUE (tenant_id, processed_item_id);
```

### 6. Update Database Setup Script

Update or create a script to apply your database schema:

```bash
#!/bin/bash

# Get the directory where this script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

# Load .env file from the parent directory
if [ -f "$SCRIPT_DIR/../../.env" ]; then
    echo "Loading environment variables from ../../.env"
    source "$SCRIPT_DIR/../../.env"
else
    echo "Error: .env file not found!"
    exit 1
fi

# Database connection string
DB_CONN="postgres://$DB_USER:$DB_PASSWORD@$DB_HOST:$DB_PORT/$DB_NAME"

# Apply the SQL file
echo "Creating your processor table..."
psql "$DB_CONN" -f "$SCRIPT_DIR/create_your_processor_table.sql"

echo "Database tables created successfully!"
```

### 7. Create the Main Go Application

Create a `main.go` file in your processor directory:

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

type Processor struct {
	db     *pgxpool.Pool
	config *Config
}

type Config struct {
	// Database config
	DBUser     string
	DBPassword string
	DBHost     string
	DBPort     string
	DBName     string
	// Tenant ID from command line
	TenantID   string
	// Your processor-specific config
	Param1     string
	Param2     string
}

func main() {
	// Define command-line flags
	tenantIDFlag := flag.String("tenant", "", "Tenant ID (required)")
	// Add your processor-specific flags
	param1Flag := flag.String("param1", "", "Parameter 1 (required)")
	param2Flag := flag.String("param2", "", "Parameter 2 (required)")

	// Parse command-line arguments
	flag.Parse()

	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	// Validate required flags
	if *tenantIDFlag == "" {
		log.Fatal("Error: tenant ID is required. Use -tenant flag")
	}

	// Initialize configuration
	config := &Config{
		DBUser:     os.Getenv("DB_USER"),
		DBPassword: os.Getenv("DB_PASSWORD"),
		DBHost:     os.Getenv("DB_HOST"),
		DBPort:     os.Getenv("DB_PORT"),
		DBName:     os.Getenv("DB_NAME"),
		TenantID:   *tenantIDFlag,
		Param1:     *param1Flag,
		Param2:     *param2Flag,
	}

	// Initialize processor
	processor, err := NewProcessor(config)
	if err != nil {
		log.Fatalf("Failed to initialize processor: %v", err)
	}
	defer processor.Close()

	// Set up graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start processing
	go processor.Start(ctx)

	// Wait for shutdown signal
	<-sigChan
	log.Println("Shutting down...")
}

func NewProcessor(config *Config) (*Processor, error) {
	// Construct Supabase connection string
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=require",
		config.DBUser, config.DBPassword, config.DBHost, config.DBPort, config.DBName)

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
		return nil, fmt.Errorf("failed to verify Supabase connection: %v", err)
	}
	log.Printf("Successfully connected to Supabase")

	// Initialize the processor
	processor := &Processor{
		db:     pool,
		config: config,
	}

	// Ensure tables exist
	if err := processor.ensureTables(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ensure tables exist: %v", err)
	}

	return processor, nil
}

func (p *Processor) ensureTables(ctx context.Context) error {
	// Create tables in a transaction
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	// Check if the tenant_id is a valid UUID
	var isValidUUID bool
	err = tx.QueryRow(ctx, `
		SELECT $1::text::uuid IS NOT NULL
	`, p.config.TenantID).Scan(&isValidUUID)

	if err != nil || !isValidUUID {
		return fmt.Errorf("invalid tenant ID format: %v", err)
	}

	// Verify tenant exists
	var tenantExists bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM tenants WHERE id = $1::uuid)
	`, p.config.TenantID).Scan(&tenantExists)

	if err != nil {
		return fmt.Errorf("failed to verify tenant existence: %v", err)
	}

	if !tenantExists {
		return fmt.Errorf("tenant with ID %s does not exist", p.config.TenantID)
	}

	// Update the tenant_services table
	_, err = tx.Exec(ctx, `
		INSERT INTO tenant_services
		(tenant_id, service_type, target_identifier, status, config, metadata)
		VALUES ($1::uuid, 'your-processor', $2, 'running', $3, $4)
		ON CONFLICT (tenant_id, service_type, target_identifier)
		DO UPDATE SET 
			status = 'running',
			updated_at = CURRENT_TIMESTAMP,
			container_id = NULL,
			metadata = $4
	`, p.config.TenantID, "some-identifier", "{}", "{}")

	if err != nil {
		return fmt.Errorf("failed to update tenant_services: %v", err)
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	return nil
}

func (p *Processor) Start(ctx context.Context) {
	log.Println("Starting data processor...")

	// Create ticker for periodic processing
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Your processing logic here
			log.Println("Processing data...")
			
			// Example: Insert processed data
			_, err := p.db.Exec(ctx, `
				INSERT INTO your_processor_table
				(tenant_id, processed_item_id, processed_data)
				VALUES ($1::uuid, $2, $3)
				ON CONFLICT (tenant_id, processed_item_id)
				DO NOTHING
			`, p.config.TenantID, "item-123", "{}")

			if err != nil {
				log.Printf("Error processing data: %v", err)
			}

		case <-ctx.Done():
			log.Println("Context cancelled, stopping processing")
			return
		}
	}
}

func (p *Processor) Close() {
	if p.db != nil {
		p.db.Close()
	}
}
```

### 8. Update Docker Compose

Add an entry to `docker-compose.yml` to build your processor image:

```yaml
# Build your processor image
your-processor-builder:
  build:
    context: ./packages/your-processor-name
    dockerfile: Dockerfile
  image: your-processor:latest
  # This service is only used to build the image, not to run any containers
  command: echo "Your Processor image built successfully"
  deploy:
    replicas: 0
```

### 9. Update Backend API

You'll need to update the backend to handle your processor's API endpoints for:

1. Processor configuration
2. Starting and stopping the processor
3. Retrieving processor status

Look at the existing `/api/v1/processors` endpoints in the backend for examples.

### 10. Update Frontend (Optional)

If you want to add UI for your processor configuration, create components in the frontend.

## Running Your Processor

Processors are typically started by the backend service when a user configures them through the UI. For manual testing, you can run:

```bash
docker run -d \
  --name your-processor-tenant-id \
  --network your_network \
  -e DB_USER=your_db_user \
  -e DB_PASSWORD=your_db_password \
  -e DB_HOST=your_db_host \
  -e DB_PORT=your_db_port \
  -e DB_NAME=your_db_name \
  your-processor:latest \
  -tenant your-tenant-id \
  -param1 value1 \
  -param2 value2
```

## Testing Your Processor

1. Build the image: `docker-compose build your-processor-builder`
2. Create a test tenant in the database
3. Run the processor with the test tenant ID
4. Check the database for processed data
