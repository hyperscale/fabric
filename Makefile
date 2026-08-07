BUILD_DIR ?= build
GO_FILES := $(shell find . -type f -name '*.go' -not -path "./vendor/*")

# Must stay in sync with the version installed by .github/workflows/go.yml.
GOLANGCI_LINT_VERSION ?= v2.12.2

# This repository is a Go workspace. `./...` only matches the root module, so
# every target below iterates over the modules listed in go.work instead.
MODULES = $(shell go list -f '{{.Dir}}/...' -m)

.PHONY: all
all: deps build test

.PHONY: deps
deps:
	@go work sync

.PHONY: clean
clean:
	@go clean -i ./...

_build:
	@mkdir -p ${BUILD_DIR}

.PHONY: build
build:
	@go build -race -v $(MODULES)

$(BUILD_DIR)/coverage.out: _build $(GO_FILES)
	@go test -count=1 -cover -race -coverprofile $(BUILD_DIR)/coverage.out.tmp -timeout 300s $(MODULES)
	@cat $(BUILD_DIR)/coverage.out.tmp | grep -v '.pb.go' | grep -v 'mock_' > $(BUILD_DIR)/coverage.out
	@rm $(BUILD_DIR)/coverage.out.tmp

.PHONY: lint
lint:
ifeq (, $(shell which golangci-lint))
	@echo "Install golangci-lint $(GOLANGCI_LINT_VERSION)..."
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
endif
	@echo "lint..."
	@golangci-lint run --timeout=300s $(MODULES)

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

.PHONY: update-go-deps
update-go-deps:
	@echo "Updating Go dependencies in all workspace modules..."
	@go list -f '{{.Dir}}' -m | while read -r dir; do \
		echo "==> $$dir"; \
		(cd "$$dir" && go get -u ./... && go mod tidy) || exit 1; \
	done
