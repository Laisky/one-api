# One-API Development Guidelines

This document provides essential development information for the One-API project, focusing on project-specific patterns, build processes, and architectural decisions that advanced developers need to know.

## Build System & Configuration

### Go Requirements

- **Go Version**: 1.25.0 (cutting-edge version required)
- **CGO**: Enabled (required for SQLite3 support)
- **Architecture**: Primarily targets AMD64, multi-arch builds supported

### Build Commands

```bash
# Backend build
go build -trimpath -ldflags "-s -w" -o one-api

# Frontend builds (multiple templates available)
make build-frontend-modern      # Primary template (React + TypeScript + Vite)
make build-frontend-air         # Alternative template
make build-frontend-berry       # Alternative template
make build-all-templates        # All templates

# Development servers
make dev-modern                 # Port 3001 (primary)
make dev-air                   # Port 3002
make dev-berry                 # Port 3003
```

### Development Tools Installation

```bash
make install    # Installs: golangci-lint, goimports, govulncheck
```

### Code Quality & Linting

```bash
make lint       # Runs complete linting pipeline:
                # - goimports with local module preference
                # - go mod tidy
                # - gofmt -s
                # - go vet
                # - golangci-lint (extensive configuration)
                # - govulncheck (security scanning)
```

## Architecture Patterns

### Relay Adapter System

The project uses a sophisticated adapter pattern for AI provider integration with 35+ supported providers:

**Core Interface** (`relay/adaptor/interface.go`):

```go
type Adaptor interface {
    Init(meta *meta.Meta)
    GetRequestURL(meta *meta.Meta) (string, error)
    SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error
    ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error)
    ConvertImageRequest(c *gin.Context, request *model.ImageRequest) (any, error)
    ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error)
    DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error)
    DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode)
    GetModelList() []string
    GetChannelName() string
    GetDefaultModelPricing() map[string]ModelConfig
    GetModelRatio(modelName string) float64
    GetCompletionRatio(modelName string) float64
}
```

### Pricing Model Architecture

Advanced pricing system supports:

- **Tiered pricing** with input token thresholds
- **Cache pricing** (5-minute and 1-hour windows)
- **Input/output ratios** for different token types
- **Image pricing** (input and output images separately)
- **Model-specific configurations** with token limits

**Example Implementation** (XAI adapter):

```go
var ModelRatios = map[string]adaptor.ModelConfig{
    "grok-4-0709": {
        Ratio: 3.0 * ratio.MilliTokensUsd,
        CompletionRatio: 5.0,
        CachedInputRatio: 0.75 * ratio.MilliTokensUsd
    },
    // Model list automatically derived from pricing map
}
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)
```

### Key Architectural Components

- **relay/**: Core relay system with adapters, billing, channels
- **controller/**: HTTP request handlers
- **model/**: Data models and database schemas
- **middleware/**: Gin middleware (auth, logging, metrics)
- **monitor/**: System monitoring and metrics
- **common/**: Shared utilities and configuration

## Database & Deployment

### Supported Databases

- **SQLite**: Default/fallback (embedded)
- **MySQL**: Production recommended (8.2.0+)
- **PostgreSQL**: Enterprise option

### Docker Deployment

Multi-stage build process:

1. **Node.js stage**: Frontend build (React/TypeScript)
2. **Go stage**: Backend compilation with SQLite3
3. **Ubuntu runtime**: With FFmpeg support for multimedia

**Key Environment Variables**:

```bash
SQL_DSN=oneapi:123456@tcp(db:3306)/one-api
REDIS_CONN_STRING=redis://redis
SESSION_SECRET=random_string
NODE_TYPE=slave                    # For multi-machine deployment
SYNC_FREQUENCY=60                  # Database sync interval
FRONTEND_BASE_URL=https://...      # For slave nodes
```

### Multi-Template Frontend System

- **Modern**: Primary template (React + TypeScript + Vite + Tailwind)
- **Air/Berry**: Alternative UI themes (no longer actively maintained)
- Each template has independent build system and development server
- Templates share the same backend API
- Note: The "default" template has been removed. Users specifying "default" are automatically redirected to "modern".

## Development Practices

### Code Organization

- **Local imports first**: `goimports -local module,github.com/songquanpeng/one-api`
- **Extensive linting**: 565-line golangci-lint configuration
- **Security scanning**: govulncheck integration
- **Dependency management**: Go modules with version constraints

### Adding New AI Providers

1. Create directory in `relay/adaptor/[provider]/`
2. Implement `Adaptor` interface
3. Define model pricing in `ModelRatios` map
4. Use `adaptor.GetModelListFromPricing()` for model list
5. Embed `DefaultPricingMethods` for standard fallbacks
6. Register adapter in relay system

### Testing Strategy

- Unit tests present (`*_test.go` files)
- API verification scripts in frontend
- Build system validates all components
- Health check endpoint: `/api/status`

## Key Dependencies

### Backend Core

- **Gin**: Web framework with comprehensive middleware
- **GORM**: ORM with multi-database support
- **Laisky libraries**: Advanced logging, middleware, utilities
- **AWS SDK v2**: Cloud provider integration
- **Prometheus**: Metrics and monitoring
- **JWT**: Authentication system

### Frontend Stack (Modern Template)

- **React 18**: Component framework
- **TypeScript**: Type safety
- **Vite**: Build system (fast HMR)
- **Tailwind CSS**: Utility-first styling
- **Yarn**: Package management

## Performance & Scaling

### Caching Strategy

- Redis for session and temporary data
- Multiple cache window pricing (5min/1hour)
- Token-level caching with custom ratios

### Monitoring

- Prometheus metrics endpoint
- Health check system
- Structured logging with Zap
- Request/response tracing

### Multi-Machine Support

- Master/slave deployment architecture
- Configurable sync frequency
- Frontend URL routing for distributed setups

---

**Note**: This project uses cutting-edge Go 1.25.0 and maintains high code quality standards through extensive tooling. The adapter pattern is the core architectural decision that enables support for 35+ AI providers while maintaining consistent pricing and request handling.
