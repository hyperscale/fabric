BUILD_DIR ?= build
GO_FILES := $(shell find . -type f -name '*.go' -not -path "./vendor/*")

.PHONY: all
all: deps build test

.PHONY: deps
deps:
	@go mod download

.PHONY: clean
clean:
	@go clean -i ./...

_build:
	@mkdir -p ${BUILD_DIR}

$(BUILD_DIR)/coverage.out: _build $(GO_FILES)
	@go test -cover -race -coverprofile $(BUILD_DIR)/coverage.out.tmp -timeout 300s ./...
	@cat $(BUILD_DIR)/coverage.out.tmp | grep -v '.pb.go' | grep -v 'mock_' > $(BUILD_DIR)/coverage.out
	@rm $(BUILD_DIR)/coverage.out.tmp

.PHONY: lint
lint:
ifeq (, $(shell which golangci-lint))
	@echo "Install golangci-lint..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.60.3
endif
	@echo "lint..."
	@golangci-lint run --timeout=300s ./...

.PHONY: test
test: $(BUILD_DIR)/coverage.out

.PHONY: coverage
coverage: $(BUILD_DIR)/coverage.out
	@echo ""
	@go tool cover -func ./$(BUILD_DIR)/coverage.out

.PHONY: coverage-html
coverage-html: $(BUILD_DIR)/coverage.out
	@go tool cover -html ./$(BUILD_DIR)/coverage.out

generate: $(GO_FILES)
	@go generate ./...

