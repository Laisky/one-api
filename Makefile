.PHONY: install
install:
	# https://golangci-lint.run/docs/welcome/install/local/
	curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(go env GOPATH)/bin v2.10.1

	go install golang.org/x/tools/cmd/goimports@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest
	# go install go.uber.org/nilaway/cmd/nilaway@latest
	# go install github.com/mitranim/gow@latest
	# go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28
	# go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2

.PHONY: lint
lint:
	goimports -local module,github.com/Laisky/one-api -w .
	go mod tidy
	gofmt -s -w .
	go vet
	# nilaway ./...
	golangci-lint run -c .golangci.yml
	govulncheck ./...
	$(MAKE) lint-goroutine-guard

# lint-goroutine-guard enforces the structural rule that no background goroutine may
# reference the request *gin.Context (gin recycles it via sync.Pool after the handler
# returns). See .ast-grep/rules/no-gin-context-in-goroutine.yml and
# docs/proposals/20260608_relay-billing-async-sync-race-fixes.md.
# Requires ast-grep (install: `pipx install ast-grep-cli`, `cargo install ast-grep --locked`,
# or a prebuilt binary from https://github.com/ast-grep/ast-grep/releases); skips gracefully
# when not installed.
.PHONY: lint-goroutine-guard
lint-goroutine-guard:
	@command -v ast-grep >/dev/null 2>&1 || { echo "ast-grep not installed; skipping goroutine *gin.Context guardrail (install: pipx install ast-grep-cli, or a prebuilt binary from https://github.com/ast-grep/ast-grep/releases)"; exit 0; }
	ast-grep test --skip-snapshot-tests
	ast-grep scan

# Development targets - Template specific
.PHONY: dev-air dev-berry dev-modern
dev-air:
	@./web/air/dev.sh dev

dev-berry:
	@./web/berry/dev.sh dev

dev-modern:
	@cd web/modern && CHOKIDAR_USEPOLLING=$${CHOKIDAR_USEPOLLING:-1} CHOKIDAR_INTERVAL=$${CHOKIDAR_INTERVAL:-1000} yarn dev

# Default dev target
.PHONY: dev
dev: dev-modern

# Build targets - Template specific
.PHONY: build-frontend-air build-frontend-berry build-frontend-modern
build-frontend-air:
	@./web/air/dev.sh build

build-frontend-berry:
	@./web/berry/dev.sh build

build-frontend-modern:
	@cd web/modern && yarn && yarn build

# Default build target
.PHONY: build-frontend
build-frontend: build-frontend-modern

# Build development versions - Template specific
.PHONY: build-frontend-dev-air build-frontend-dev-berry build-frontend-dev-modern
build-frontend-dev-air:
	@./web/air/dev.sh build-dev

build-frontend-dev-berry:
	@./web/berry/dev.sh build-dev

build-frontend-dev-modern:
	@cd web/modern && npm run build

# Default dev build target
.PHONY: build-frontend-dev
build-frontend-dev: build-frontend-dev-modern

# Build all templates
.PHONY: build-all-templates
build-all-templates: build-frontend-air build-frontend-berry build-frontend-modern

# Help target
.PHONY: help-dev
help-dev:
	@echo "Development targets:"
	@echo "  dev-air           Start air template development server (port 3002)"
	@echo "  dev-berry         Start berry template development server (port 3003)"
	@echo "  dev-modern        Start modern template development server (port 3001)"
	@echo "  dev               Start modern template development server (default)"
	@echo ""
	@echo "Build targets:"
	@echo "  build-frontend-air         Build air template for production"
	@echo "  build-frontend-berry       Build berry template for production"
	@echo "  build-frontend-modern      Build modern template for production"
	@echo "  build-all-templates        Build all templates for production"
	@echo ""
	@echo "Development build targets:"
	@echo "  build-frontend-dev-air     Build air template for development"
	@echo "  build-frontend-dev-berry   Build berry template for development"
	@echo "  build-frontend-dev-modern  Build modern template for development"
