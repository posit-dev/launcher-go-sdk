# Launcher Go SDK justfile
# Run `just` or `just --list` to see available commands

# Default recipe - shows help
default:
    @just --list

# Run all tests
test:
    go test ./...

# Run tests with verbose output
test-verbose:
    go test -v ./...

# Run tests with race detector
test-race:
    go test -race ./...

# Run tests with coverage report
test-coverage:
    go test ./... -coverprofile=coverage.out
    go tool cover -func=coverage.out

# Run tests and open coverage report in browser
test-coverage-html:
    go test ./... -coverprofile=coverage.out
    go tool cover -html=coverage.out

# Run tests for a specific package
test-package package:
    go test -v ./{{package}}

# Run linter
lint:
    golangci-lint run

# Run linter with auto-fix where possible
lint-fix:
    golangci-lint run --fix

# Format all Go code
fmt:
    go fmt ./...
    @if command -v goimports >/dev/null 2>&1; then \
        goimports -w -local github.com/posit-dev/launcher-go-sdk .; \
    else \
        echo "goimports not found in PATH, skipping import formatting"; \
        echo "Install with: go install golang.org/x/tools/cmd/goimports@latest"; \
        echo "Add $(go env GOPATH)/bin to your PATH"; \
    fi

# Check if code is formatted
fmt-check:
    @if [ -n "$(gofmt -l .)" ]; then \
        echo "The following files are not formatted:"; \
        gofmt -l .; \
        exit 1; \
    fi

# Build all packages
build:
    go build ./...

# Build all examples
build-examples:
    @echo "Building inmemory example..."
    cd examples/inmemory && go build

# Build a specific example
build-example name:
    cd examples/{{name}} && go build

# Clean build artifacts and test outputs
clean:
    rm -f coverage.out
    find . -name "*.test" -delete
    find examples -type f -executable -delete

# Verify dependencies
verify:
    go mod verify

# Tidy dependencies
tidy:
    go mod tidy

# Download dependencies
download:
    go mod download

# Run all checks (format, lint, test)
check: fmt-check lint test

# Run all checks and build everything
ci: check build build-examples

# Show test coverage by package
coverage-by-package:
    go test ./... -coverprofile=coverage.out
    @echo "\nCoverage by package:"
    @go tool cover -func=coverage.out | grep -E '^github.com' | column -t

# Run benchmarks
bench:
    go test -bench=. -benchmem ./...

# Run benchmarks for a specific package
bench-package package:
    go test -bench=. -benchmem ./{{package}}

# Check for outdated dependencies
outdated:
    go list -u -m all

# Install development tools
install-tools:
    @echo "Installing golangci-lint..."
    @which golangci-lint > /dev/null || go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
    @echo "Installing goimports..."
    @which goimports > /dev/null || go install golang.org/x/tools/cmd/goimports@latest
    @echo "Tools installed successfully"

# Generate documentation
docs:
    @echo "Generating godoc documentation..."
    @echo "Run 'godoc -http=:6060' and visit http://localhost:6060/pkg/github.com/posit-dev/launcher-go-sdk/"
    godoc -http=:6060

# Run security check
security:
    @which govulncheck > /dev/null || go install golang.org/x/vuln/cmd/govulncheck@latest
    govulncheck ./...

# View package dependencies as a graph
deps-graph:
    @which go-mod-graph-chart > /dev/null || go install github.com/nikolaydubina/go-mod-graph-chart@latest
    go mod graph | go-mod-graph-chart

# Pre-commit checks (fast)
pre-commit: fmt lint

# Pre-push checks (comprehensive)
pre-push: check build-examples
