#!/bin/bash

# One-API Template-Specific Development Helper Script
# This script helps streamline the development workflow for a specific template

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Template configuration
TEMPLATE_NAME="air"
TEMPLATE_PORT="3002"
FRONTEND_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${FRONTEND_DIR}/../.." && pwd)"

print_help() {
    echo -e "${BLUE}One-API Development Helper - ${TEMPLATE_NAME} Template${NC}"
    echo ""
    echo "Usage: $0 [COMMAND]"
    echo ""
    echo "Commands:"
    echo "  dev        Start frontend development server with HMR"
    echo "  build      Build frontend for production"
    echo "  build-dev  Build frontend for development (with sourcemaps)"
    echo "  help       Show this help message"
    echo ""
    echo "Development workflow for ${TEMPLATE_NAME} template:"
    echo "1. Start your Go backend server: go run main.go"
    echo "2. In another terminal, run: $0 dev"
    echo "3. Open browser to http://localhost:${TEMPLATE_PORT} for frontend with HMR"
    echo "4. Make frontend changes - they'll update instantly!"
    echo "5. Backend API calls will be proxied to your Go server automatically"
}

start_dev() {
    echo -e "${GREEN}Starting ${TEMPLATE_NAME} template development server...${NC}"
    echo -e "${YELLOW}Frontend will be available at: http://localhost:${TEMPLATE_PORT}${NC}"
    echo -e "${YELLOW}API calls will be proxied to: http://100.113.170.10:3000${NC}"
    echo ""
    cd "${FRONTEND_DIR}"
    export REACT_APP_VERSION=$(date +%s)
    yarn run dev:backend
}

build_prod() {
    echo -e "${GREEN}Building ${TEMPLATE_NAME} template for production...${NC}"
    cd "${FRONTEND_DIR}"
    yarn run build:prod
    echo -e "${GREEN}${TEMPLATE_NAME} template built successfully!${NC}"
}

build_dev() {
    echo -e "${GREEN}Building ${TEMPLATE_NAME} template for development...${NC}"
    cd "${FRONTEND_DIR}"
    yarn run build:dev
    echo -e "${GREEN}${TEMPLATE_NAME} template built successfully!${NC}"
}

# Main script logic
case "${1:-help}" in
    "dev")
        start_dev
        ;;
    "build")
        build_prod
        ;;
    "build-dev")
        build_dev
        ;;
    "help"|"--help"|"-h")
        print_help
        ;;
    *)
        echo -e "${RED}Unknown command: $1${NC}"
        print_help
        exit 1
        ;;
esac
