# EVM Indexer

This is a Docker-based EVM contract event indexer that runs as a tenant-specific job. It indexes events from smart contracts on EVM-compatible blockchains and stores them in a database.

## Features

- Multi-tenant architecture: Each tenant gets its own indexer instance
- Automatically finds the first transaction on a contract as the starting block
- Stores events in a tenant-specific table
- Designed to run as a Docker container

## Usage

### Building the Docker Image

```bash
# From the project root directory
docker-compose build evm-indexer-builder
```

### Database Setup

First, set up the necessary database tables:

```bash
# From the project root directory
chmod +x packages/backend/db/apply_evm_indexer_tables.sh
packages/backend/db/apply_evm_indexer_tables.sh
```

### Starting the Indexer from the Frontend

1. Register a new account and tenant
2. Go to the data sources setup page
3. Add a smart contract by providing:
   - Contract address (e.g. `0x1234567890abcdef1234567890abcdef12345678`)
   - Contract ABI (JSON format)
   - Start block (optional - will be auto-detected if not provided)
4. Click "Continue to Dashboard" to start the indexing process

### Checking Indexer Status

The dashboard page shows all active indexers for your tenant, including:
- Current processing status
- Block progress
- Contract address

## Command Line Usage

You can also run the indexer directly with Docker:

```bash
docker run -d \
  --name evm-indexer-your-tenant-id \
  --network your_network \
  -e DB_USER=your_db_user \
  -e DB_PASSWORD=your_db_password \
  -e DB_HOST=your_db_host \
  -e DB_PORT=your_db_port \
  -e DB_NAME=your_db_name \
  -e RPC_URL=your_rpc_url \
  -e INFURA_API_KEY=your_infura_key \
  -e INFURA_API_SECRET=your_infura_secret \
  evm-indexer:latest \
  -tenant your-tenant-id \
  -contract 0x1234567890abcdef1234567890abcdef12345678 \
  -abi '[{"type":"event","name":"Transfer","inputs":[{"indexed":true,"name":"from"},{"indexed":true,"name":"to"},{"indexed":false,"name":"value"}]}]'
```

## API Endpoints

The backend provides the following API endpoints for managing indexers:

- `POST /api/v1/contracts` - Register a new contract for indexing
- `POST /api/v1/processors` - Get all processors for a tenant
- `POST /api/v1/processors/start` - Start all processors for a tenant
- `POST /api/v1/processors/stop` - Stop all processors for a tenant 