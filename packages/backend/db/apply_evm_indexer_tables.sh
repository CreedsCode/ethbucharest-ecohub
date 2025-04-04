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

# Apply the SQL files
echo "Creating evm_indexer table..."
psql "$DB_CONN" -f "$SCRIPT_DIR/create_evm_indexer_table.sql"

echo "Creating tenant_services table..."
psql "$DB_CONN" -f "$SCRIPT_DIR/create_tenant_services_table.sql"

echo "Database tables created successfully!" 