package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/dwarvesf/icy-backend/internal/utils/logger"
)

// Simple working tests to verify basic functionality
func TestHealthHandler_Basic_Simple(t *testing.T) {
	// Arrange
	gin.SetMode(gin.TestMode)
	
	handler := &HealthHandler{}
	
	router := gin.New()
	router.GET("/healthz", handler.Basic)

	// Act
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/healthz", nil)
	
	start := time.Now()
	router.ServeHTTP(w, req)
	duration := time.Since(start)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, duration < 200*time.Millisecond, 
		"Basic health check exceeded SLA: %v", duration)

	var response BasicHealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "ok", response.Message)
}

func TestHealthHandler_Database_NilDB(t *testing.T) {
	// Arrange
	gin.SetMode(gin.TestMode)
	
	handler := &HealthHandler{
		db:     nil,
		logger: logger.New("test"),
	}
	
	router := gin.New()
	router.GET("/health/db", handler.Database)

	// Act
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health/db", nil)
	
	start := time.Now()
	router.ServeHTTP(w, req)
	duration := time.Since(start)

	// Assert
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.True(t, duration < 500*time.Millisecond, 
		"Database health check exceeded SLA: %v", duration)

	var response HealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "unhealthy", response.Status)
	assert.Contains(t, response.Checks, "database")
	
	dbCheck := response.Checks["database"]
	assert.Equal(t, "unhealthy", dbCheck.Status)
	assert.Contains(t, dbCheck.Error, "database connection not available")
}

func TestHealthHandler_External_NilRPCs(t *testing.T) {
	// Arrange
	gin.SetMode(gin.TestMode)
	
	handler := &HealthHandler{
		btcRPC:  nil,
		baseRPC: nil,
		logger:  logger.New("test"),
	}
	
	router := gin.New()
	router.GET("/health/external", handler.External)

	// Act
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health/external", nil)
	
	start := time.Now()
	router.ServeHTTP(w, req)
	duration := time.Since(start)

	// Assert
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.True(t, duration < 2000*time.Millisecond, 
		"External API health check exceeded SLA: %v", duration)

	var response HealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "unhealthy", response.Status)
	assert.Contains(t, response.Checks, "blockstream_api")
	assert.Contains(t, response.Checks, "base_rpc")
	
	btcCheck := response.Checks["blockstream_api"]
	assert.Equal(t, "unhealthy", btcCheck.Status)
	assert.Contains(t, btcCheck.Error, "bitcoin rpc not available")
	
	baseCheck := response.Checks["base_rpc"]
	assert.Equal(t, "unhealthy", baseCheck.Status)
	assert.Contains(t, baseCheck.Error, "base rpc not available")
}

func TestHealthHandler_ResponseFormat_Basic(t *testing.T) {
	// Arrange
	gin.SetMode(gin.TestMode)
	
	handler := &HealthHandler{}
	
	router := gin.New()
	router.GET("/healthz", handler.Basic)

	// Act
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/healthz", nil)
	router.ServeHTTP(w, req)

	// Assert response format
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	
	expectedFields := []string{"message"}
	for _, field := range expectedFields {
		assert.Contains(t, response, field, 
			"Missing required field: %s", field)
	}
}

func TestHealthHandler_ResponseFormat_Database(t *testing.T) {
	// Arrange
	gin.SetMode(gin.TestMode)
	
	handler := &HealthHandler{
		db:     nil, // Will make it unhealthy, but test response format
		logger: logger.New("test"),
	}
	
	router := gin.New()
	router.GET("/health/db", handler.Database)

	// Act
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health/db", nil)
	router.ServeHTTP(w, req)

	// Assert response format
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	
	expectedFields := []string{"status", "timestamp", "checks", "duration_ms"}
	for _, field := range expectedFields {
		assert.Contains(t, response, field, 
			"Missing required field: %s", field)
	}
}

func TestHealthHandler_ResponseFormat_External(t *testing.T) {
	// Arrange
	gin.SetMode(gin.TestMode)
	
	handler := &HealthHandler{
		btcRPC:  nil, // Will make it unhealthy, but test response format
		baseRPC: nil,
		logger:  logger.New("test"),
	}
	
	router := gin.New()
	router.GET("/health/external", handler.External)

	// Act
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health/external", nil)
	router.ServeHTTP(w, req)

	// Assert response format
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	
	expectedFields := []string{"status", "timestamp", "checks", "duration_ms"}
	for _, field := range expectedFields {
		assert.Contains(t, response, field, 
			"Missing required field: %s", field)
	}
}