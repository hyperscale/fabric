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
	@go list -f '{{.Dir}}/...' -m | xargs go test -count=1 -cover -race -coverprofile $(BUILD_DIR)/coverage.out.tmp -timeout 300s
	@cat $(BUILD_DIR)/coverage.out.tmp | grep -v '.pb.go' | grep -v 'mock_' > $(BUILD_DIR)/coverage.out
	@rm $(BUILD_DIR)/coverage.out.tmp

.PHONY: lint
lint:
ifeq (, $(shell which golangci-lint))
	@echo "Install golangci-lint..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.60.3
endif
	@echo "lint..."
	@go list -f '{{.Dir}}/...' -m | xargs golangci-lint run --timeout=300s

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
