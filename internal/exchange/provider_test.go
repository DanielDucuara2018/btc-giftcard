package exchange

import (
	"btc-giftcard/pkg/logger"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	// Initialize logger for tests
	_ = logger.Init("development")
}

func TestNewProvider(t *testing.T) {
	tests := []struct {
		name        string
		provider    string
		expectError bool
	}{
		{"Coinbase lowercase", "coinbase", false},
		{"Coinbase uppercase", "COINBASE", false},
		{"CoinGecko lowercase", "coingecko", false},
		{"CoinGecko mixed case", "CoinGecko", false},
		{"Bitstamp lowercase", "bitstamp", false},
		{"Unknown provider", "unknown", true},
		{"Empty string", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use empty baseURL and nil client for production defaults
			provider, err := NewProvider(tt.provider, "", nil)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, provider)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, provider)
			}
		})
	}
}

func TestCoinbase_GetPrice_Success(t *testing.T) {
	// Create mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request path
		assert.Equal(t, "/v2/prices/BTC-USD/spot", r.URL.Path)

		// Return mock response
		response := coinbasePriceResponse{
			Data: struct {
				Amount   string `json:"amount"`
				Base     string `json:"base"`
				Currency string `json:"currency"`
			}{
				Amount:   "67000.50",
				Base:     "BTC",
				Currency: "USD",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create provider with mock server URL
	provider, err := NewProvider("coinbase", server.URL, server.Client())
	require.NoError(t, err)

	ctx := context.Background()
	price, err := provider.GetPrice(ctx, "USD")

	require.NoError(t, err)
	assert.Equal(t, 67000.50, price)
}

func TestCoinbase_GetPrice_MockServer(t *testing.T) {
	tests := []struct {
		name           string
		currency       string
		mockResponse   interface{}
		mockStatusCode int
		expectError    bool
		expectedPrice  float64
	}{
		{
			name:     "Valid USD price",
			currency: "USD",
			mockResponse: coinbasePriceResponse{
				Data: struct {
					Amount   string `json:"amount"`
					Base     string `json:"base"`
					Currency string `json:"currency"`
				}{
					Amount:   "67000.50",
					Base:     "BTC",
					Currency: "USD",
				},
			},
			mockStatusCode: http.StatusOK,
			expectError:    false,
			expectedPrice:  67000.50,
		},
		{
			name:     "Valid EUR price",
			currency: "EUR",
			mockResponse: coinbasePriceResponse{
				Data: struct {
					Amount   string `json:"amount"`
					Base     string `json:"base"`
					Currency string `json:"currency"`
				}{
					Amount:   "62000.00",
					Base:     "BTC",
					Currency: "EUR",
				},
			},
			mockStatusCode: http.StatusOK,
			expectError:    false,
			expectedPrice:  62000.00,
		},
		{
			name:           "API returns 500 error",
			currency:       "USD",
			mockResponse:   map[string]string{"error": "Internal server error"},
			mockStatusCode: http.StatusInternalServerError,
			expectError:    true,
		},
		{
			name:           "API returns 404 error",
			currency:       "USD",
			mockResponse:   map[string]string{"error": "Not found"},
			mockStatusCode: http.StatusNotFound,
			expectError:    true,
		},
		{
			name:           "Invalid JSON response",
			currency:       "USD",
			mockResponse:   "invalid json",
			mockStatusCode: http.StatusOK,
			expectError:    true,
		},
		{
			name:     "Zero price value",
			currency: "USD",
			mockResponse: coinbasePriceResponse{
				Data: struct {
					Amount   string `json:"amount"`
					Base     string `json:"base"`
					Currency string `json:"currency"`
				}{
					Amount:   "0",
					Base:     "BTC",
					Currency: "USD",
				},
			},
			mockStatusCode: http.StatusOK,
			expectError:    true, // Should fail validation
		},
		{
			name:     "Negative price value",
			currency: "USD",
			mockResponse: coinbasePriceResponse{
				Data: struct {
					Amount   string `json:"amount"`
					Base     string `json:"base"`
					Currency string `json:"currency"`
				}{
					Amount:   "-100",
					Base:     "BTC",
					Currency: "USD",
				},
			},
			mockStatusCode: http.StatusOK,
			expectError:    true, // Should fail validation
		},
		{
			name:     "Invalid number format",
			currency: "USD",
			mockResponse: coinbasePriceResponse{
				Data: struct {
					Amount   string `json:"amount"`
					Base     string `json:"base"`
					Currency string `json:"currency"`
				}{
					Amount:   "not-a-number",
					Base:     "BTC",
					Currency: "USD",
				},
			},
			mockStatusCode: http.StatusOK,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.mockStatusCode)

				if str, ok := tt.mockResponse.(string); ok {
					w.Write([]byte(str))
				} else {
					json.NewEncoder(w).Encode(tt.mockResponse)
				}
			}))
			defer server.Close()

			// Create provider with mock URL
			provider, err := NewProvider("coinbase", server.URL, server.Client())
			require.NoError(t, err)

			ctx := context.Background()
			price, err := provider.GetPrice(ctx, tt.currency)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedPrice, price)
			}
		})
	}
}

func TestCoingecko_GetPrice_Success(t *testing.T) {
	// Create mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "/api/v3/simple/price", r.URL.Path)
		assert.Equal(t, "bitcoin", r.URL.Query().Get("ids"))
		assert.Equal(t, "usd", r.URL.Query().Get("vs_currencies"))

		// Return mock response
		response := coingeckoPriceResponse{
			"bitcoin": {
				"usd": 67500.00,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider, err := NewProvider("coingecko", server.URL, server.Client())
	require.NoError(t, err)

	ctx := context.Background()
	price, err := provider.GetPrice(ctx, "USD")

	require.NoError(t, err)
	assert.Equal(t, 67500.00, price)
}

func TestCoingecko_GetPrice_Errors(t *testing.T) {
	tests := []struct {
		name         string
		currency     string
		mockResponse interface{}
		errorContain string
	}{
		{
			name:     "Currency not in response",
			currency: "JPY",
			mockResponse: coingeckoPriceResponse{
				"bitcoin": {
					"usd": 67500.00,
				},
			},
			errorContain: "currency jpy not found",
		},
		{
			name:         "Bitcoin not in response",
			currency:     "USD",
			mockResponse: coingeckoPriceResponse{},
			errorContain: "currency usd not found",
		},
		{
			name:     "Zero price",
			currency: "USD",
			mockResponse: coingeckoPriceResponse{
				"bitcoin": {
					"usd": 0,
				},
			},
			errorContain: "invalid price value",
		},
		{
			name:     "Negative price",
			currency: "USD",
			mockResponse: coingeckoPriceResponse{
				"bitcoin": {
					"usd": -100,
				},
			},
			errorContain: "invalid price value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(tt.mockResponse)
			}))
			defer server.Close()

			provider, err := NewProvider("coingecko", server.URL, server.Client())
			require.NoError(t, err)

			ctx := context.Background()
			price, err := provider.GetPrice(ctx, tt.currency)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorContain)
			assert.Equal(t, 0.0, price)
		})
	}
}

func TestBitstamp_GetPrice_Success(t *testing.T) {
	// Create mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request path
		assert.Equal(t, "/api/v2/ticker/btcusd", r.URL.Path)

		// Return mock response
		response := bitstampPriceResponse{
			Last: "67250.50",
			Ask:  "67251.00",
			Bid:  "67250.00",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider, err := NewProvider("bitstamp", server.URL, server.Client())
	require.NoError(t, err)

	ctx := context.Background()
	price, err := provider.GetPrice(ctx, "USD")

	require.NoError(t, err)
	assert.Equal(t, 67250.50, price)
}

func TestBitstamp_GetPrice_Errors(t *testing.T) {
	tests := []struct {
		name         string
		mockResponse interface{}
		errorContain string
	}{
		{
			name: "Invalid price format",
			mockResponse: bitstampPriceResponse{
				Last: "invalid",
			},
			errorContain: "invalid price format",
		},
		{
			name: "Zero price",
			mockResponse: bitstampPriceResponse{
				Last: "0",
			},
			errorContain: "invalid price value",
		},
		{
			name: "Negative price",
			mockResponse: bitstampPriceResponse{
				Last: "-100.50",
			},
			errorContain: "invalid price value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(tt.mockResponse)
			}))
			defer server.Close()

			provider, err := NewProvider("bitstamp", server.URL, server.Client())
			require.NoError(t, err)

			ctx := context.Background()
			price, err := provider.GetPrice(ctx, "USD")

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorContain)
			assert.Equal(t, 0.0, price)
		})
	}
}

func TestNewProvider_CustomURL(t *testing.T) {
	tests := []struct {
		name        string
		provider    string
		baseURL     string
		httpClient  *http.Client
		expectError bool
	}{
		{
			name:        "Valid coinbase with custom URL",
			provider:    "coinbase",
			baseURL:     "https://custom.api.com",
			httpClient:  &http.Client{Timeout: 5 * time.Second},
			expectError: false,
		},
		{
			name:        "Valid with nil client (uses default)",
			provider:    "coingecko",
			baseURL:     "https://custom.api.com",
			httpClient:  nil,
			expectError: false,
		},
		{
			name:        "Empty baseURL uses production",
			provider:    "coinbase",
			baseURL:     "",
			httpClient:  nil,
			expectError: false,
		},
		{
			name:        "Unknown provider with custom URL",
			provider:    "unknown",
			baseURL:     "https://api.com",
			httpClient:  nil,
			expectError: true,
		},
		{
			name:        "Case insensitive provider name",
			provider:    "BITSTAMP",
			baseURL:     "https://api.com",
			httpClient:  nil,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewProvider(tt.provider, tt.baseURL, tt.httpClient)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, provider)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, provider)
			}
		})
	}
}

func TestFetchJSON_Success(t *testing.T) {
	// Create mock server with valid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
	}))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	ctx := context.Background()

	var result map[string]string
	err := fetchJSON(ctx, client, server.URL, &result)

	require.NoError(t, err)
	assert.Equal(t, "success", result["status"])
}

func TestFetchJSON_HTTPError(t *testing.T) {
	// Create mock server that returns 500 error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	ctx := context.Background()

	var result map[string]string
	err := fetchJSON(ctx, client, server.URL, &result)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "API error: status 500")
}

func TestFetchJSON_InvalidJSON(t *testing.T) {
	// Create mock server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("invalid json {{{"))
	}))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	ctx := context.Background()

	var result map[string]string
	err := fetchJSON(ctx, client, server.URL, &result)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
}

func TestFetchJSON_ContextCancellation(t *testing.T) {
	// Create mock server with delay
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	var result map[string]string
	err := fetchJSON(ctx, client, server.URL, &result)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}

func TestFetchJSON_NetworkError(t *testing.T) {
	client := &http.Client{Timeout: 1 * time.Second}
	ctx := context.Background()

	// Use invalid URL
	var result map[string]string
	err := fetchJSON(ctx, client, "http://invalid-host-that-does-not-exist-12345.com", &result)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch data")
}

// Integration tests - these hit real APIs and should be run manually or in integration test suite
// Use build tag: go test -tags=integration
func TestCoinbase_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	provider, err := NewProvider("coinbase", "", nil)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	price, err := provider.GetPrice(ctx, "USD")
	require.NoError(t, err)
	assert.Greater(t, price, 0.0)
	assert.Less(t, price, 1000000.0) // Sanity check

	t.Logf("Current BTC price (Coinbase): $%.2f", price)
}

func TestCoingecko_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	provider, err := NewProvider("coingecko", "", nil)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	price, err := provider.GetPrice(ctx, "USD")
	require.NoError(t, err)
	assert.Greater(t, price, 0.0)
	assert.Less(t, price, 1000000.0)

	t.Logf("Current BTC price (CoinGecko): $%.2f", price)
}

func TestBitstamp_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	provider, err := NewProvider("bitstamp", "", nil)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	price, err := provider.GetPrice(ctx, "USD")
	require.NoError(t, err)
	assert.Greater(t, price, 0.0)
	assert.Less(t, price, 1000000.0)

	t.Logf("Current BTC price (Bitstamp): $%.2f", price)
}

func TestAllProviders_ConsistentPrices(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	providers := []string{"coinbase", "coingecko", "bitstamp"}
	prices := make(map[string]float64)

	for _, providerName := range providers {
		provider, err := NewProvider(providerName, "", nil)
		require.NoError(t, err)

		price, err := provider.GetPrice(ctx, "USD")
		require.NoError(t, err)
		prices[providerName] = price

		t.Logf("%s: $%.2f", providerName, price)
	}

	// Prices should be within 5% of each other
	var min, max float64
	for _, price := range prices {
		if min == 0 || price < min {
			min = price
		}
		if price > max {
			max = price
		}
	}

	percentDiff := ((max - min) / min) * 100
	assert.Less(t, percentDiff, 5.0, "Prices differ by more than 5%%")
}
