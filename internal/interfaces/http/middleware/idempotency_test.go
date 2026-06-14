package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"payment-kita.backend/pkg/redis"
)

func setupTestRouter() *gin.Engine {
	router := gin.New()
	router.Use(IdempotencyMiddleware())
	router.POST("/test-payment", func(c *gin.Context) {
		// Simulate payment creation
		time.Sleep(10 * time.Millisecond) // Simulate processing time
		c.JSON(http.StatusCreated, gin.H{
			"payment_id": "pay_123",
			"status":     "created",
			"amount":     "100.00",
		})
	})
	return router
}

func TestIdempotencyMiddleware_PreventsDuplicateRequests(t *testing.T) {
	// Setup miniredis
	mr, err := miniredis.Run()
	assert.NoError(t, err)
	defer mr.Close()

	// Initialize redis with miniredis
	err = redis.Init("redis://"+mr.Addr(), "", "")
	assert.NoError(t, err)

	router := setupTestRouter()

	// First request
	req1, _ := http.NewRequest("POST", "/test-payment", bytes.NewBuffer([]byte(`{"amount": "100.00"}`)))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set(IdempotencyKeyHeader, "idem_key_123")
	req1.Header.Set("X-PK-Merchant-ID", "merchant_123")

	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)

	// Second request with same idempotency key (should return cached response)
	req2, _ := http.NewRequest("POST", "/test-payment", bytes.NewBuffer([]byte(`{"amount": "100.00"}`)))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set(IdempotencyKeyHeader, "idem_key_123")
	req2.Header.Set("X-PK-Merchant-ID", "merchant_123")

	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	// Both should return same response
	assert.Equal(t, http.StatusCreated, w1.Code)
	assert.Equal(t, http.StatusCreated, w2.Code) // Cached response also returns 201

	var resp1, resp2 map[string]interface{}
	json.Unmarshal(w1.Body.Bytes(), &resp1)
	json.Unmarshal(w2.Body.Bytes(), &resp2)

	assert.Equal(t, resp1["payment_id"], resp2["payment_id"])
	assert.Equal(t, resp1["status"], resp2["status"])
}

func TestIdempotencyMiddleware_AllowsDifferentKeys(t *testing.T) {
	// Setup miniredis
	mr, err := miniredis.Run()
	assert.NoError(t, err)
	defer mr.Close()

	// Initialize redis with miniredis
	err = redis.Init("redis://"+mr.Addr(), "", "")
	assert.NoError(t, err)

	router := setupTestRouter()

	// First request
	req1, _ := http.NewRequest("POST", "/test-payment", bytes.NewBuffer([]byte(`{"amount": "100.00"}`)))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set(IdempotencyKeyHeader, "idem_key_A")
	req1.Header.Set("X-PK-Merchant-ID", "merchant_123")

	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)

	// Second request with different idempotency key (should process normally)
	req2, _ := http.NewRequest("POST", "/test-payment", bytes.NewBuffer([]byte(`{"amount": "200.00"}`)))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set(IdempotencyKeyHeader, "idem_key_B")
	req2.Header.Set("X-PK-Merchant-ID", "merchant_123")

	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	// Both should be processed
	assert.Equal(t, http.StatusCreated, w1.Code)
	assert.Equal(t, http.StatusCreated, w2.Code)
}

func TestIdempotencyMiddleware_NoKeyContinuesNormally(t *testing.T) {
	// Setup miniredis
	mr, err := miniredis.Run()
	assert.NoError(t, err)
	defer mr.Close()

	// Initialize redis with miniredis
	err = redis.Init("redis://"+mr.Addr(), "", "")
	assert.NoError(t, err)

	router := setupTestRouter()

	// Request without idempotency key
	req, _ := http.NewRequest("POST", "/test-payment", bytes.NewBuffer([]byte(`{"amount": "100.00"}`)))
	req.Header.Set("Content-Type", "application/json")
	// No idempotency key header

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should process normally
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestIdempotencyMiddleware_RejectsTooLongKey(t *testing.T) {
	// Setup miniredis
	mr, err := miniredis.Run()
	assert.NoError(t, err)
	defer mr.Close()

	// Initialize redis with miniredis
	err = redis.Init("redis://"+mr.Addr(), "", "")
	assert.NoError(t, err)

	router := setupTestRouter()

	// Request with too long key
	longKey := string(make([]byte, MaxIdempotencyKeyLength+1))
	req, _ := http.NewRequest("POST", "/test-payment", bytes.NewBuffer([]byte(`{"amount": "100.00"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(IdempotencyKeyHeader, longKey)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should reject
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestValidateIdempotencyKey(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		expectError bool
	}{
		{"Valid key", "idem_key_123", false},
		{"Valid key with dashes", "idem-key-123", false},
		{"Valid key with underscores", "idem_key_123", false},
		{"Empty key (valid)", "", false},
		{"Too long key", string(make([]byte, MaxIdempotencyKeyLength+1)), true},
		{"Invalid character (space)", "idem key", true},
		{"Invalid character (special)", "idem@key", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIdempotencyKey(tt.key)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGenerateIdempotencyKey(t *testing.T) {
	key1 := GenerateIdempotencyKey()
	key2 := GenerateIdempotencyKey()

	// Should generate unique keys
	assert.NotEqual(t, key1, key2)

	// Should have correct prefix
	assert.Contains(t, key1, "idem_")
	assert.Contains(t, key2, "idem_")

	// Should be valid
	assert.NoError(t, ValidateIdempotencyKey(key1))
	assert.NoError(t, ValidateIdempotencyKey(key2))
}
