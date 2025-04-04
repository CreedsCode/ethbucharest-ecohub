-- Create a generalized table to track tenant service runners (Docker jobs)
CREATE TABLE IF NOT EXISTS tenant_services (
    id SERIAL PRIMARY KEY,
    tenant_id UUID NOT NULL,
    service_type VARCHAR(50) NOT NULL, -- e.g., 'evm-indexer', 'data-processor', etc.
    container_id VARCHAR(100), -- Docker container ID if running
    target_identifier VARCHAR(100), -- Contract address, API endpoint, etc.
    last_processed_id VARCHAR(100), -- Could be a block number, timestamp, or any identifier
    status VARCHAR(20) NOT NULL, -- 'running', 'stopped', 'failed', etc.
    config JSONB, -- Configuration parameters
    metadata JSONB, -- Additional metadata like statistics, performance metrics, etc.
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Add indexes to improve query performance
CREATE INDEX IF NOT EXISTS idx_tenant_services_tenant_id ON tenant_services(tenant_id);
CREATE INDEX IF NOT EXISTS idx_tenant_services_status ON tenant_services(status);
CREATE INDEX IF NOT EXISTS idx_tenant_services_service_type ON tenant_services(service_type);
CREATE INDEX IF NOT EXISTS idx_tenant_services_target_identifier ON tenant_services(target_identifier);

-- Create a unique constraint on tenant_id, service_type, and target_identifier
ALTER TABLE tenant_services ADD CONSTRAINT unique_tenant_service 
    UNIQUE (tenant_id, service_type, target_identifier); 