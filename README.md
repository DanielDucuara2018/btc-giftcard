# BTC Gift Card - Bitcoin Gift Card Platform

A Bitcoin gift card system that allows users to purchase, store, and redeem Bitcoin through digital gift cards.

---

## Quick Start

### Initialize Project

```bash
# Initialize Go module (first time only)
go mod init btc-giftcard

# Install dependencies
go mod download

# Clean up unused dependencies
go mod tidy
```

### Install Dependencies

```bash
# Install specific packages
go get go.uber.org/zap                    # Logger
go get github.com/redis/go-redis/v9       # Redis client
go get golang.org/x/crypto/argon2         # Encryption
```

---

## Running the Application

### Run Without Compiling

```bash
# Run API server
go run ./cmd/api

# Run with environment variable
ENVIRONMENT=production go run ./cmd/api
ENVIRONMENT=development go run ./cmd/api
```

### Compile and Run

```bash
# Build binary
go build -o bin/api cmd/api/main.go

# Run binary
./bin/api
```

---

## Testing

### Run Tests

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test ./... -v

# Run specific package tests
go test ./internal/crypto -v
go test ./pkg/logger -v
go test ./pkg/cache -v

# Run specific test function
go test ./internal/crypto -run TestEncryptDecrypt -v
```

### Test Coverage

```bash
# Show coverage percentage
go test ./internal/crypto -cover

# Generate detailed coverage report
go test ./internal/crypto -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Benchmarks

```bash
# Run all benchmarks
go test ./internal/crypto -bench=.

# Run with memory stats
go test ./internal/crypto -bench=. -benchmem

# Run specific benchmark
go test ./internal/crypto -bench=BenchmarkEncrypt
```

---

## Code Quality

### Format Code

```bash
# Format all files
go fmt ./...

# Check for common mistakes
go vet ./...
```

---

## Docker (Redis)

### Start Redis

```bash
# Start Redis with Docker Compose
docker-compose up -d

# View Redis logs
docker logs btc-giftcard-redis-1
```

### Stop Services

```bash
docker-compose down
```

---

## Project Structure

```
btc-giftcard/
├── cmd/
│   ├── api/              # HTTP API server
│   ├── worker/           # Background job processor
│   └── migrate/          # Database migrations
├── internal/
│   ├── card/            # Gift card business logic
│   ├── wallet/          # Bitcoin wallet operations
│   ├── crypto/          # Encryption/decryption
│   ├── exchange/        # Exchange integrations
│   ├── payment/         # Payment processing
│   └── database/        # Database layer
├── pkg/
│   ├── cache/           # Redis wrapper
│   ├── queue/           # RabbitMQ wrapper
│   └── logger/          # Logging utilities
└── config/              # Configuration files
```

---

## Environment Variables

```bash
# Application
ENVIRONMENT=development        # or production

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0
```

---

## Common Go Commands

```bash
# Get help on any command
go help [command]

# List all dependencies
go list -m all

# Update specific dependency
go get -u github.com/redis/go-redis/v9

# Remove unused dependencies
go mod tidy

# Download all dependencies
go mod download
```

---

## Development Workflow

1. **Write code** in appropriate module
2. **Format**: `go fmt ./...`
3. **Check errors**: `go vet ./...`
4. **Write tests**: Create `*_test.go` files
5. **Run tests**: `go test ./... -v`
6. **Run application**: `go run ./cmd/api`

---

## Useful Tips

- Go automatically downloads dependencies on first `go run` or `go build`
- Test files must end with `_test.go`
- `internal/` packages are private to this project
- `pkg/` packages can be imported by external projects
- Use `-v` flag for verbose output in tests
- Use `go test -run TestName` to run specific tests only
