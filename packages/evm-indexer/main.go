package main

import (
	"context"
	"encoding/json"
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
}

// Custom HTTP client with authentication
type authTransport struct {
	key    string
	secret string
	base   http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.SetBasicAuth(t.key, t.secret)
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
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Fatal("Error loading .env file")
	}

	// Initialize configuration
	startBlock, err := strconv.ParseUint(os.Getenv("START_BLOCK"), 10, 64)
	if err != nil {
		log.Fatal("Invalid START_BLOCK value in .env file")
	}

	config := &Config{
		RPCURL:          fmt.Sprintf("%s%s", os.Getenv("RPC_URL"), os.Getenv("INFURA_API_KEY")),
		RPCKey:          os.Getenv("INFURA_API_KEY"),
		RPCSecret:       os.Getenv("INFURA_API_SECRET"),
		DBUser:          os.Getenv("DB_USER"),
		DBPassword:      os.Getenv("DB_PASSWORD"),
		DBHost:          os.Getenv("DB_HOST"),
		DBPort:          os.Getenv("DB_PORT"),
		DBName:          os.Getenv("DB_NAME"),
		ContractAddress: os.Getenv("CONTRACT_ADDRESS"),
		ContractABI:     os.Getenv("CONTRACT_ABI"),
		StartBlock:      startBlock,
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
		return nil, fmt.Errorf("failed to connect to Base Sepolia: %v", err)
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

	log.Printf("Connected to Base Sepolia. Current block: %d", blockNumber)

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

	// Create txs table
	_, err = tx.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS txs (
			id SERIAL PRIMARY KEY,
			block_number BIGINT NOT NULL,
			transaction_hash VARCHAR(66) NOT NULL,
			event_type VARCHAR(50) NOT NULL,
			contract_address VARCHAR(42) NOT NULL,
			event_data JSONB NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create txs table: %v", err)
	}

	// Add unique constraint if it doesn't exist
	_, err = tx.Exec(ctx, `
		DO $$ 
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint 
				WHERE conname = 'txs_transaction_hash_key'
			) THEN
				ALTER TABLE txs ADD CONSTRAINT txs_transaction_hash_key UNIQUE (transaction_hash);
			END IF;
		END $$;
	`)
	if err != nil {
		return fmt.Errorf("failed to add unique constraint: %v", err)
	}

	// Create indexes
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_txs_block_number ON txs(block_number)",
		"CREATE INDEX IF NOT EXISTS idx_txs_transaction_hash ON txs(transaction_hash)",
		"CREATE INDEX IF NOT EXISTS idx_txs_event_type ON txs(event_type)",
		"CREATE INDEX IF NOT EXISTS idx_txs_contract_address ON txs(contract_address)",
		"CREATE INDEX IF NOT EXISTS idx_txs_event_data ON txs USING GIN (event_data)",
	}

	for _, index := range indexes {
		_, err = tx.Exec(ctx, index)
		if err != nil {
			return fmt.Errorf("failed to create index: %v", err)
		}
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	return nil
}

func (i *Indexer) Start(ctx context.Context) {
	log.Println("Starting Base Sepolia indexer...")

	// Ensure tables exist
	if err := i.ensureTables(ctx); err != nil {
		log.Printf("Warning: %v", err)
	}

	// Get the latest processed block from database
	var lastProcessedBlock uint64
	err := i.db.QueryRow(ctx, "SELECT COALESCE(MAX(block_number), 0) FROM txs").Scan(&lastProcessedBlock)
	if err != nil {
		log.Printf("Failed to get last processed block: %v", err)
		lastProcessedBlock = i.config.StartBlock
	}

	// Always start from the configured START_BLOCK if it's earlier than the last processed block
	if i.config.StartBlock < lastProcessedBlock {
		log.Printf("Overriding last processed block %d with configured START_BLOCK %d",
			lastProcessedBlock, i.config.StartBlock)
		lastProcessedBlock = i.config.StartBlock
	}

	log.Printf("Starting from block %d", lastProcessedBlock)

	// Create ticker for polling with a longer interval
	ticker := time.NewTicker(30 * time.Second) // Increased to 30 seconds to be more conservative
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

					// Store event in database
					result, err := i.db.Exec(ctx, `
						INSERT INTO txs 
						(block_number, transaction_hash, event_type, contract_address, event_data)
						VALUES ($1, $2, $3, $4, $5)
						ON CONFLICT (transaction_hash) DO NOTHING
					`, vLog.BlockNumber, vLog.TxHash.Hex(), event.Name,
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

				// Update last processed block
				lastProcessedBlock = endBlock
				log.Printf("Processed blocks %d-%d, found %d events (%d new, %d skipped)",
					startBlock, endBlock, len(logs), newEvents, skippedEvents)

				// Add a longer delay between batches to avoid rate limiting
				time.Sleep(500 * time.Millisecond)
			}

		case <-ctx.Done():
			return
		}
	}
}

func (i *Indexer) subscribeToContractEvents(ctx context.Context) {
	contractAddress := common.HexToAddress(i.config.ContractAddress)
	contractABI, err := abi.JSON(strings.NewReader(i.config.ContractABI))
	if err != nil {
		log.Printf("Failed to parse contract ABI: %v", err)
		return
	}

	// Create a filter query starting from the specified block
	query := ethereum.FilterQuery{
		Addresses: []common.Address{contractAddress},
		FromBlock: big.NewInt(int64(i.config.StartBlock)),
	}

	// Subscribe to logs
	logs := make(chan types.Log)
	sub, err := i.client.SubscribeFilterLogs(ctx, query, logs)
	if err != nil {
		log.Printf("Failed to subscribe to contract events: %v", err)
		return
	}
	defer sub.Unsubscribe()

	for {
		select {
		case err := <-sub.Err():
			log.Printf("Contract event subscription error: %v", err)
			// Attempt to reconnect after a delay
			time.Sleep(5 * time.Second)
			sub, err = i.client.SubscribeFilterLogs(ctx, query, logs)
			if err != nil {
				log.Printf("Failed to resubscribe to contract events: %v", err)
				continue
			}
		case vLog := <-logs:
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

			// Store event in database
			result, err := i.db.Exec(ctx, `
				INSERT INTO txs 
				(block_number, transaction_hash, event_type, contract_address, event_data)
				VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT (transaction_hash) DO NOTHING
			`, vLog.BlockNumber, vLog.TxHash.Hex(), event.Name,
				vLog.Address.Hex(), eventDataJSON)
			if err != nil {
				log.Printf("Failed to store %s event: %v", event.Name, err)
				continue
			}

			// Log whether the event was new or skipped
			if result.RowsAffected() > 0 {
				log.Printf("New event stored: %s (tx: %s)", event.Name, vLog.TxHash.Hex())
			} else {
				log.Printf("Skipped duplicate event: %s (tx: %s)", event.Name, vLog.TxHash.Hex())
			}
		case <-ctx.Done():
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
