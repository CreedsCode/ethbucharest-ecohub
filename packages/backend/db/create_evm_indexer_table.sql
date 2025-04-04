-- Create the evm_indexer table to store indexed transactions by tenant
CREATE TABLE IF NOT EXISTS evm_indexer (
    id SERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL,
    block_number BIGINT NOT NULL,
    transaction_hash VARCHAR(66) NOT NULL,
    event_type VARCHAR(50) NOT NULL,
    contract_address VARCHAR(42) NOT NULL,
    event_data JSONB NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Add indexes to improve query performance
CREATE INDEX IF NOT EXISTS idx_evm_indexer_tenant_id ON evm_indexer(tenant_id);
CREATE INDEX IF NOT EXISTS idx_evm_indexer_block_number ON evm_indexer(block_number);
CREATE INDEX IF NOT EXISTS idx_evm_indexer_transaction_hash ON evm_indexer(transaction_hash);
CREATE INDEX IF NOT EXISTS idx_evm_indexer_event_type ON evm_indexer(event_type);
CREATE INDEX IF NOT EXISTS idx_evm_indexer_contract_address ON evm_indexer(contract_address);
CREATE INDEX IF NOT EXISTS idx_evm_indexer_event_data ON evm_indexer USING GIN (event_data);

-- Create a unique constraint on tenant_id and transaction_hash
ALTER TABLE evm_indexer ADD CONSTRAINT unique_tenant_tx UNIQUE (tenant_id, transaction_hash); 