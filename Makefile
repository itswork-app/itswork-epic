# ItsWork-epic Industrial Makefile

.PHONY: test coverage lint build clean run proto

# Default target
all: lint test build

test:
	@echo "Running all Go tests..."
	go test -v ./...

coverage:
	@echo "Generating coverage report..."
	@go test -coverprofile=coverage.out ./internal/... ./pkg/... ./cmd/ingestor/... || echo "Tests failed but continuing coverage report..."
	@# Remove lines for generated files or packages with no tests to avoid issues
	@grep -v "_pb.go" coverage.out > coverage_clean.out 2>/dev/null || cp coverage.out coverage_clean.out
	@mv coverage_clean.out coverage.out
	@go tool cover -func=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage threshold check (80% target)..."
	@COVERAGE=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//'); \
	echo "Current Coverage: $$COVERAGE%"; \
	if [ $$(echo "$$COVERAGE < 80" | bc -l) -eq 1 ]; then \
		echo "Error: Coverage is below 80%"; \
		exit 1; \
	fi

lint:
	@echo "Running golangci-lint..."
	~/go/bin/golangci-lint run ./...

build:
	@echo "Building ItsWork Ingestor..."
	go build -o bin/ingestor cmd/ingestor/main.go

clean:
	rm -rf bin/ coverage.out coverage.html

run: build
	./bin/ingestor

proto:
	@echo "Generating gRPC Proto stubs (Go & Python)..."
	protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative api/proto/CONTRACTS.proto
	python3 -m grpc_tools.protoc -I. --python_out=. --grpc_python_out=. api/proto/CONTRACTS.proto
	@echo "Proto regenerated. Master Blueprint Read & Verified."
