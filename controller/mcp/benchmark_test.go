package mcp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/common/config"
)

// Benchmark the handler performance
func BenchmarkHandler(b *testing.B) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/mcp", Handler)

	for b.Loop() {
		req, _ := http.NewRequest("GET", "/mcp", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

// Benchmark test for server creation
func BenchmarkNewServer(b *testing.B) {
	for b.Loop() {
		server := NewServer()
		_ = server // Avoid unused variable warning
	}
}

// Benchmark test for getBaseURL
func BenchmarkGetBaseURL(b *testing.B) {
	config.ServerAddress = "https://api.example.com"

	for b.Loop() {
		url := getBaseURL()
		_ = url // Avoid unused variable warning
	}
}
