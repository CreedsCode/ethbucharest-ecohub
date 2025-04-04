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
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o evm-indexer .

# Create a minimal production image
FROM debian:bookworm-slim

# Add ca-certificates for secure connections
RUN apt-get update && apt-get install -y ca-certificates tzdata && rm -rf /var/lib/apt/lists/*

WORKDIR /root/

# Copy the binary from the builder stage
COPY --from=builder /app/evm-indexer .

# Make the binary executable
RUN chmod +x ./evm-indexer

# Create a non-root user to run the application
RUN useradd -ms /bin/bash appuser
USER appuser

# Command to run the executable
ENTRYPOINT ["./evm-indexer"] 