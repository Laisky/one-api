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
	goimports -local module,github.com/songquanpeng/one-api -w .
	go mod tidy
	gofmt -s -w .
	go vet
	# nilaway ./...
	golangci-lint run -c .golangci.yml
	govulncheck ./...

# Development targets - Template specific
.PHONY: dev-default dev-air dev-berry dev-modern
dev-default:
	@./web/default/dev.sh dev

dev-air:
	@./web/air/dev.sh dev

dev-berry:
	@./web/berry/dev.sh dev

dev-modern:
	@cd web/modern && npm run dev

# Legacy dev target (defaults to default template)
.PHONY: dev
dev: dev-modern

# Build targets - Template specific
.PHONY: build-frontend-default build-frontend-air build-frontend-berry build-frontend-modern
build-frontend-default:
	@./web/default/dev.sh build

build-frontend-air:
	@./web/air/dev.sh build

build-frontend-berry:
	@./web/berry/dev.sh build

build-frontend-modern:
	@cd web/modern && yarn && yarn build

# Legacy build target (builds default template)
.PHONY: build-frontend
build-frontend: build-frontend-modern

# Build development versions - Template specific
.PHONY: build-frontend-dev-default build-frontend-dev-air build-frontend-dev-berry build-frontend-dev-modern
build-frontend-dev-default:
	@./web/default/dev.sh build-dev

build-frontend-dev-air:
	@./web/air/dev.sh build-dev

build-frontend-dev-berry:
	@./web/berry/dev.sh build-dev

build-frontend-dev-modern:
	@cd web/modern && npm run build

# Legacy dev build target (builds default template)
.PHONY: build-frontend-dev
build-frontend-dev: build-frontend-dev-default

# Build all templates
.PHONY: build-all-templates
build-all-templates: build-frontend-default build-frontend-air build-frontend-berry build-frontend-modern

# Help target
.PHONY: help-dev
help-dev:
	@echo "Development targets:"
	@echo "  dev-default       Start default template development server (port 3001)"
	@echo "  dev-air           Start air template development server (port 3002)"
	@echo "  dev-berry         Start berry template development server (port 3003)"
	@echo "  dev-modern        Start modern template development server (port 3001)"
	@echo "  dev               Start default template development server (legacy)"
	@echo ""
	@echo "Build targets:"
	@echo "  build-frontend-default     Build default template for production"
	@echo "  build-frontend-air         Build air template for production"
	@echo "  build-frontend-berry       Build berry template for production"
	@echo "  build-frontend-modern      Build modern template for production"
	@echo "  build-all-templates        Build all templates for production"
	@echo ""
	@echo "Development build targets:"
	@echo "  build-frontend-dev-default Build default template for development"
	@echo "  build-frontend-dev-air     Build air template for development"
	@echo "  build-frontend-dev-berry   Build berry template for development"
	@echo "  build-frontend-dev-modern  Build modern template for development"
