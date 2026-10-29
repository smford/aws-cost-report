BINARY_NAME := aws-cost-report
MODULE      := github.com/smford/aws-cost-report
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE        ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS     := -s -w \
               -X main.version=$(VERSION) \
               -X main.commit=$(COMMIT) \
               -X main.date=$(DATE)

.PHONY: all help build build-linux-arm64 build-linux-amd64 clean test lint pre-commit run-mock run-compare run-services install

all: build

help:
	@echo "aws-cost-report developer targets:"
	@echo "  build              Build static binary for local host"
	@echo "  build-linux-arm64  Cross-compile static binary for linux/arm64"
	@echo "  build-linux-amd64  Cross-compile static binary for linux/amd64"
	@echo "  test               Run test suite with race detector (-race)"
	@echo "  lint               Verify code formatting (gofmt), go vet, and modules"
	@echo "  pre-commit         Run pre-commit hooks on all files"
	@echo "  run-mock           Run aws-cost-report generate with simulated mock data"
	@echo "  run-compare        Run aws-cost-report generate with Month-over-Month comparison"
	@echo "  run-services       List official AWS Cost Explorer service names (mock)"
	@echo "  install            Install binary to \$$GOPATH/bin"
	@echo "  clean              Remove build artifacts and temporary binaries"

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) ./cmd/aws-cost-report

build-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME)-linux-arm64 ./cmd/aws-cost-report

build-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME)-linux-amd64 ./cmd/aws-cost-report

test:
	go test -v -race ./...

lint:
	@echo "Checking formatting (gofmt -s -l)..."
	@unformatted=$$(gofmt -s -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "Unformatted files found:\n$$unformatted" >&2; \
		exit 1; \
	fi
	@echo "Running go vet..."
	go vet ./...
	@echo "Verifying modules..."
	go mod verify

pre-commit:
	pre-commit run --all-files

run-mock: build
	./$(BINARY_NAME) generate --mock

run-compare: build
	./$(BINARY_NAME) generate --mock --compare

run-services: build
	./$(BINARY_NAME) services --mock

install:
	CGO_ENABLED=0 go install -ldflags "$(LDFLAGS)" ./cmd/aws-cost-report

clean:
	rm -f $(BINARY_NAME) $(BINARY_NAME)-linux-arm64 $(BINARY_NAME)-linux-amd64 coverage.out coverage.html
	rm -rf dist/
