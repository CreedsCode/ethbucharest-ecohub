package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/big"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

type Indexer struct {
	client *ethclient.Client
	db     *pgxpool.Pool
	config *Config
}

type Config struct {
	RPCURL          string
	RPCKey          string
	RPCSecret       string
	DBUser          string
	DBPassword      string
	DBHost          string
	DBPort          string
	DBName          string
	ContractAddress string
	ContractABI     string
	StartBlock      uint64
	TenantID        string
}

// Custom HTTP client with authentication
type authTransport struct {
	key    string
	secret string
	base   http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Use an empty username with the secret as the password
	req.SetBasicAuth("", t.secret)
	return t.base.RoundTrip(req)
}

func NewAuthClient(url, key, secret string) (*ethclient.Client, error) {
	// Create a custom HTTP client with authentication
	transport := &authTransport{
		key:    key,
		secret: secret,
		base:   http.DefaultTransport,
	}
	httpClient := &http.Client{Transport: transport}

	// Create a custom RPC client with the authenticated HTTP client
	rpcClient, err := rpc.DialHTTPWithClient(url, httpClient)
	if err != nil {
		return nil, err
	}

	// Create the Ethereum client using the custom RPC client
	return ethclient.NewClient(rpcClient), nil
}

func main() {
	// Define command-line flags
	tenantIDFlag := flag.String("tenant", "", "Tenant ID (required)")
	contractAddressFlag := flag.String("contract", "", "Contract address to index (required)")
	contractABIFlag := flag.String("abi", "", "Contract ABI JSON string (required)")
	startBlockFlag := flag.Uint64("startBlock", 0, "Starting block number (required)")

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

	if *contractAddressFlag == "" {
		log.Fatal("Error: contract address is required. Use -contract flag")
	}

	if *contractABIFlag == "" {
		log.Fatal("Error: contract ABI is required. Use -abi flag")
	}

	if *startBlockFlag == 0 {
		log.Fatal("Error: start block is required. Use -startBlock flag")
	}

	// Initialize configuration
	config := &Config{
		RPCURL:          os.Getenv("RPC_URL"),
		RPCKey:          os.Getenv("INFURA_API_KEY"),
		RPCSecret:       os.Getenv("INFURA_API_SECRET"),
		DBUser:          os.Getenv("DB_USER"),
		DBPassword:      os.Getenv("DB_PASSWORD"),
		DBHost:          os.Getenv("DB_HOST"),
		DBPort:          os.Getenv("DB_PORT"),
		DBName:          os.Getenv("DB_NAME"),
		ContractAddress: *contractAddressFlag,
		ContractABI:     *contractABIFlag,
		StartBlock:      *startBlockFlag,
		TenantID:        *tenantIDFlag,
	}

	// Initialize indexer
	indexer, err := NewIndexer(config)
	if err != nil {
		log.Fatalf("Failed to initialize indexer: %v", err)
	}
	defer indexer.Close()

	// Set up graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start indexing
	go indexer.Start(ctx)

	// Wait for shutdown signal
	<-sigChan
	log.Println("Shutting down...")
}

func NewIndexer(config *Config) (*Indexer, error) {
	// Initialize Ethereum client with HTTP and API key authentication
	client, err := NewAuthClient(config.RPCURL, config.RPCKey, config.RPCSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to blockchain: %v", err)
	}

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

	// Test database connection with a simple query
	var result int
	err = pool.QueryRow(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to verify Supabase connection: %v", err)
	}
	log.Printf("Successfully connected to Supabase")

	// Test table creation
	indexer := &Indexer{
		client: client,
		db:     pool,
		config: config,
	}

	if err := indexer.ensureTables(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ensure tables exist: %v", err)
	}
	log.Printf("Successfully verified database schema")

	// Test Ethereum connection with retry logic
	var blockNumber uint64
	for retry := 0; retry < 3; retry++ {
		blockNumber, err = client.BlockNumber(ctx)
		if err == nil {
			break
		}

		// Check if it's a rate limit error
		if strings.Contains(err.Error(), "Too Many Requests") {
			delay := time.Duration(1<<uint(retry)) * time.Second
			log.Printf("Rate limited, waiting %v before retry", delay)
			time.Sleep(delay)
			continue
		}

		pool.Close()
		return nil, fmt.Errorf("failed to get latest block number: %v", err)
	}

	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to get latest block number after retries: %v", err)
	}

	log.Printf("Connected to blockchain. Current block: %d", blockNumber)

	return &Indexer{
		client: client,
		db:     pool,
		config: config,
	}, nil
}

func (i *Indexer) ensureTables(ctx context.Context) error {
	// Create tables in a transaction
	tx, err := i.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	// Check if the tenant_id is a valid UUID
	var isValidUUID bool
	err = tx.QueryRow(ctx, `
		SELECT $1::text::uuid IS NOT NULL
	`, i.config.TenantID).Scan(&isValidUUID)

	if err != nil || !isValidUUID {
		return fmt.Errorf("invalid tenant ID format: %v", err)
	}

	// Verify tenant exists
	var tenantExists bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM tenants WHERE id = $1::uuid)
	`, i.config.TenantID).Scan(&tenantExists)

	if err != nil {
		return fmt.Errorf("failed to verify tenant existence: %v", err)
	}

	if !tenantExists {
		return fmt.Errorf("tenant with ID %s does not exist", i.config.TenantID)
	}

	// Update the tenant_services table to mark this service as running
	_, err = tx.Exec(ctx, `
		INSERT INTO tenant_services
		(tenant_id, service_type, target_identifier, last_processed_id, status, config, metadata)
		VALUES ($1::uuid, 'evm-indexer', $2, $3, 'running', $4, jsonb_build_object('start_time', to_char(NOW(), 'YYYY-MM-DD"T"HH24:MI:SS"Z"')))
		ON CONFLICT (tenant_id, service_type, target_identifier)
		DO UPDATE SET 
			status = 'running',
			updated_at = CURRENT_TIMESTAMP,
			container_id = NULL,
			metadata = jsonb_build_object('start_time', to_char(NOW(), 'YYYY-MM-DD"T"HH24:MI:SS"Z"'))
	`, i.config.TenantID, i.config.ContractAddress, strconv.FormatUint(i.config.StartBlock, 10),
		json.RawMessage(`{"contract_abi": "configured"}`))

	if err != nil {
		return fmt.Errorf("failed to update tenant_services: %v", err)
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	return nil
}

func (i *Indexer) Start(ctx context.Context) {
	log.Println("Starting blockchain indexer...")

	// Get the latest processed block from database for this tenant and contract
	var lastProcessedBlock uint64
	err := i.db.QueryRow(ctx, `
		SELECT COALESCE(MAX(block_number), 0) 
		FROM evm_indexer 
		WHERE tenant_id = $1::uuid AND contract_address = $2
	`, i.config.TenantID, i.config.ContractAddress).Scan(&lastProcessedBlock)

	if err != nil {
		log.Printf("Failed to get last processed block: %v", err)

		// Also check tenant_services table for last_processed_id
		var lastProcessedID string
		err = i.db.QueryRow(ctx, `
			SELECT COALESCE(last_processed_id, '0')
			FROM tenant_services
			WHERE tenant_id = $1::uuid 
			  AND service_type = 'evm-indexer'
			  AND target_identifier = $2
		`, i.config.TenantID, i.config.ContractAddress).Scan(&lastProcessedID)

		if err != nil {
			log.Printf("Also failed to get last processed ID from tenant_services: %v", err)
			lastProcessedBlock = i.config.StartBlock
		} else {
			// Convert last processed ID to uint64 if possible
			if val, err := strconv.ParseUint(lastProcessedID, 10, 64); err == nil {
				lastProcessedBlock = val
			} else {
				lastProcessedBlock = i.config.StartBlock
			}
		}
	}

	// Start from the greater of last processed block or configured start block
	if lastProcessedBlock < i.config.StartBlock {
		lastProcessedBlock = i.config.StartBlock
	}

	log.Printf("Starting from block %d", lastProcessedBlock)

	// Create ticker for polling with a longer interval
	ticker := time.NewTicker(30 * time.Second) // 30 seconds to be conservative
	defer ticker.Stop()

	// Parse contract ABI once
	contractABI, err := abi.JSON(strings.NewReader(i.config.ContractABI))
	if err != nil {
		log.Printf("Failed to parse contract ABI: %v", err)
		return
	}

	contractAddress := common.HexToAddress(i.config.ContractAddress)

	// Rate limiting variables
	var (
		rateLimitDelay    = 2 * time.Second
		maxDelay          = 60 * time.Second
		consecutiveErrors = 0
		batchSize         = 50 // Reduced batch size to be more conservative
	)

	for {
		select {
		case <-ticker.C:
			// Get latest block number with retry logic
			var latestBlock uint64
			for retry := 0; retry < 3; retry++ {
				latestBlock, err = i.client.BlockNumber(ctx)
				if err == nil {
					consecutiveErrors = 0
					rateLimitDelay = 2 * time.Second
					break
				}

				if strings.Contains(err.Error(), "Too Many Requests") {
					consecutiveErrors++
					delay := rateLimitDelay * time.Duration(1<<uint(consecutiveErrors-1))
					if delay > maxDelay {
						delay = maxDelay
					}
					delay = delay + time.Duration(rand.Int63n(int64(delay/2)))
					log.Printf("Rate limited, waiting %v before retry", delay)
					time.Sleep(delay)
					continue
				}

				log.Printf("Failed to get latest block number: %v", err)
				break
			}

			if err != nil {
				log.Printf("Failed to get latest block number after retries: %v", err)
				continue
			}

			// Process blocks in smaller batches to avoid rate limiting
			for startBlock := lastProcessedBlock + 1; startBlock <= latestBlock; startBlock += uint64(batchSize) {
				endBlock := startBlock + uint64(batchSize-1)
				if endBlock > latestBlock {
					endBlock = latestBlock
				}

				// Create filter query
				query := ethereum.FilterQuery{
					FromBlock: big.NewInt(int64(startBlock)),
					ToBlock:   big.NewInt(int64(endBlock)),
					Addresses: []common.Address{contractAddress},
				}

				// Get logs with retry logic
				var logs []types.Log
				for retry := 0; retry < 3; retry++ {
					logs, err = i.client.FilterLogs(ctx, query)
					if err == nil {
						consecutiveErrors = 0
						rateLimitDelay = 2 * time.Second
						break
					}

					if strings.Contains(err.Error(), "Too Many Requests") {
						consecutiveErrors++
						delay := rateLimitDelay * time.Duration(1<<uint(consecutiveErrors-1))
						if delay > maxDelay {
							delay = maxDelay
						}
						delay = delay + time.Duration(rand.Int63n(int64(delay/2)))
						log.Printf("Rate limited, waiting %v before retry", delay)
						time.Sleep(delay)
						continue
					}

					log.Printf("Failed to get logs for blocks %d-%d: %v", startBlock, endBlock, err)
					break
				}

				if err != nil {
					log.Printf("Failed to get logs for blocks %d-%d after retries: %v", startBlock, endBlock, err)
					continue
				}

				// Process logs
				var newEvents, skippedEvents int
				for _, vLog := range logs {
					// Process the log
					event, err := contractABI.EventByID(vLog.Topics[0])
					if err != nil {
						log.Printf("Failed to get event from log: %v", err)
						continue
					}

					// Create a map to store event data
					eventData := make(map[string]interface{})

					// Unpack all event data into the map
					err = contractABI.UnpackIntoMap(eventData, event.Name, vLog.Data)
					if err != nil {
						log.Printf("Failed to unpack %s event: %v", event.Name, err)
						continue
					}

					// Convert event data to JSON
					eventDataJSON, err := json.Marshal(eventData)
					if err != nil {
						log.Printf("Failed to marshal event data to JSON: %v", err)
						continue
					}

					// Store event in database - now using evm_indexer table with tenant_id
					result, err := i.db.Exec(ctx, `
						INSERT INTO evm_indexer 
						(tenant_id, block_number, transaction_hash, event_type, contract_address, event_data)
						VALUES ($1::uuid, $2, $3, $4, $5, $6)
						ON CONFLICT (tenant_id, transaction_hash) DO NOTHING
					`, i.config.TenantID, vLog.BlockNumber, vLog.TxHash.Hex(), event.Name,
						vLog.Address.Hex(), eventDataJSON)
					if err != nil {
						log.Printf("Failed to store %s event: %v", event.Name, err)
						continue
					}

					// Count new vs skipped events
					if result.RowsAffected() > 0 {
						newEvents++
					} else {
						skippedEvents++
					}
				}

				// After processing logs, update the last processed block
				_, err = i.db.Exec(ctx, `
					UPDATE tenant_services
					SET last_processed_id = $1,
						updated_at = CURRENT_TIMESTAMP,
						metadata = jsonb_set(COALESCE(metadata, '{}'::jsonb), '{last_update}', $2::jsonb)
					WHERE tenant_id = $3::uuid
					AND service_type = 'evm-indexer'
					AND target_identifier = $4
				`, strconv.FormatUint(endBlock, 10),
					json.RawMessage(fmt.Sprintf(`"%s"`, time.Now().Format(time.RFC3339))),
					i.config.TenantID, i.config.ContractAddress)

				if err != nil {
					log.Printf("Failed to update last processed ID: %v", err)
				}

				// Update last processed block
				lastProcessedBlock = endBlock
				log.Printf("Processed blocks %d-%d, found %d events (%d new, %d skipped)",
					startBlock, endBlock, len(logs), newEvents, skippedEvents)

				// Add a longer delay between batches to avoid rate limiting
				time.Sleep(500 * time.Millisecond)
			}

		case <-ctx.Done():
			// Update status to stopped when shutting down
			_, err := i.db.Exec(ctx, `
				UPDATE tenant_services
				SET status = 'stopped',
					updated_at = CURRENT_TIMESTAMP,
					metadata = jsonb_set(
						COALESCE(metadata, '{}'::jsonb), 
						'{stop_time}', 
						to_jsonb(to_char(NOW(), 'YYYY-MM-DD"T"HH24:MI:SS"Z"')::text)
					)
				WHERE tenant_id = $1::uuid
				AND service_type = 'evm-indexer'
				AND target_identifier = $2
			`, i.config.TenantID, i.config.ContractAddress)

			if err != nil {
				log.Printf("Failed to update service status to stopped: %v", err)
			}

			return
		}
	}
}

func (i *Indexer) Close() {
	if i.client != nil {
		i.client.Close()
	}
	if i.db != nil {
		i.db.Close()
	}
}
