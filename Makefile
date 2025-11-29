# Makefile for go-pflow

.PHONY: help build test test-coverage clean install fmt vet lint examples run-basic run-neural run-monitoring rebuild-all-svg check all

# Default target
help:
	@echo "Available targets:"
	@echo "  make build           - Build the pflow CLI tool"
	@echo "  make test            - Run all tests"
	@echo "  make test-coverage   - Run tests with coverage report"
	@echo "  make clean           - Remove build artifacts and generated files"
	@echo "  make install         - Install the pflow CLI tool"
	@echo "  make fmt             - Format all Go code"
	@echo "  make vet             - Run go vet on all packages"
	@echo "  make lint            - Run static analysis (requires golangci-lint)"
	@echo "  make check           - Run fmt, vet, and tests"
	@echo "  make all             - Run check and build"
	@echo "  make examples        - Build all example programs"
	@echo "  make run-basic       - Run basic example"
	@echo "  make run-neural      - Run neural ODE example"
	@echo "  make run-monitoring  - Run monitoring demo"
	@echo "  make rebuild-all-svg - Regenerate all SVG visualizations"

# Build the main CLI tool
build:
	@echo "Building pflow CLI..."
	go build -o bin/pflow ./cmd/pflow

# Run all tests
test:
	@echo "Running tests..."
	go test ./... -v

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test ./... -coverprofile=coverage.txt -covermode=atomic
	go tool cover -func=coverage.txt

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -f coverage.txt
	rm -f **/*.svg
	rm -f **/*.png
	rm -f **/results.json
	go clean ./...

# Install the CLI tool
install:
	@echo "Installing pflow CLI..."
	go install ./cmd/pflow

# Format all Go code
fmt:
	@echo "Formatting Go code..."
	go fmt ./...

# Run go vet
vet:
	@echo "Running go vet..."
	go vet ./...

# Run static analysis (requires golangci-lint)
lint:
	@echo "Running golangci-lint..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not found. Install it from https://golangci-lint.run/"; \
	fi

# Check code quality
check: fmt vet test

# Build everything
all: check build

# Build all example programs
examples:
	@echo "Building examples..."
	@echo "  - Basic example"
	@go build -o bin/basic examples/basic/main.go
	@echo "  - Event log demo"
	@go build -o bin/eventlog_demo examples/eventlog_demo/main.go
	@echo "  - Mining demo"
	@go build -o bin/mining_demo examples/mining_demo/main.go
	@echo "  - Monitoring demo"
	@go build -o bin/monitoring_demo examples/monitoring_demo/main.go
	@echo "  - Incident simulator"
	@go build -o bin/incident_simulator examples/incident_simulator/main.go
	@echo "  - Neural ODE examples"
	@go build -o bin/neural_demo examples/neural/cmd/demo/main.go
	@go build -o bin/neural_main examples/neural/cmd/main/main.go
	@go build -o bin/neural_reverse examples/neural/cmd/reverse/main.go
	@echo "  - Dataset comparison examples"
	@go build -o bin/synthetic_sir examples/dataset_comparison/cmd/synthetic_sir/main.go
	@go build -o bin/measles_sir examples/dataset_comparison/cmd/measles_sir/main.go
	@go build -o bin/measles_sir_fixed examples/dataset_comparison/cmd/measles_sir_fixed/main.go
	@go build -o bin/covid_seir examples/dataset_comparison/cmd/covid_seir/main.go
	@echo "  - Game AI examples"
	@go build -o bin/tictactoe examples/tictactoe/cmd/*.go
	@go build -o bin/nim examples/nim/cmd/*.go
	@go build -o bin/connect4 examples/connect4/cmd/*.go
	@echo "  - Chess problem examples"
	@go build -o bin/chess examples/chess/cmd/*.go
	@echo "Done building examples!"

# Run basic example
run-basic:
	@echo "Running basic example..."
	@go run examples/basic/main.go

# Run neural ODE example
run-neural:
	@echo "Running neural ODE example..."
	@go run examples/neural/cmd/main/main.go

# Run monitoring demo
run-monitoring:
	@echo "Running monitoring demo..."
	@go run examples/monitoring_demo/main.go

# Rebuild all SVG visualizations
rebuild-all-svg:
	@echo "Regenerating all SVG visualizations..."
	@echo ""
	@echo "=== Basic Examples ==="
	@cd examples/basic && go run main.go
	@echo ""
	@echo "=== Neural ODE Examples ==="
	@cd examples/neural/cmd/main && go run main.go
	@cd examples/neural/cmd/reverse && go run main.go
	@echo ""
	@echo "=== Dataset Comparison Examples ==="
	@cd examples/dataset_comparison && go run cmd/synthetic_sir/main.go
	@cd examples/dataset_comparison && go run cmd/measles_sir/main.go
	@cd examples/dataset_comparison && go run cmd/measles_sir_fixed/main.go
	@cd examples/dataset_comparison && go run cmd/covid_seir/main.go
	@echo ""
	@echo "=== Process Mining Examples ==="
	@cd examples/eventlog_demo && go run main.go
	@cd examples/mining_demo && go run main.go
	@cd examples/monitoring_demo && go run main.go
	@cd examples/incident_simulator && go run main.go --regression-test
	@echo ""
	@echo "=== Game AI Examples ==="
	@cd examples/tictactoe/cmd && go run *.go
	@cd examples/nim/cmd && go run *.go --player-x=ode --player-o=optimal
	@cd examples/connect4/cmd && go run *.go --player-x=ode --player-o=random
	@echo ""
	@echo "=== Sudoku Example ==="
	@cd examples/sudoku/cmd && go run *.go
	@echo ""
	@echo "✓ All SVG files regenerated!"

# Quick check before publishing
publish-check: clean check
	@echo "Running full build test..."
	@go build ./...
	@echo ""
	@echo "✓ All checks passed! Repository is ready for publishing."
	@echo ""
	@echo "Next steps:"
	@echo "  1. git add ."
	@echo "  2. git commit -m 'Prepare for publication'"
	@echo "  3. git push"
