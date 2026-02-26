//go:build integration

package lnd

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"btc-giftcard/pkg/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	_ = logger.Init("development")
}

// ============================================================================
// Integration tests — require a running LND container
// Run with: go test -tags=integration ./internal/lnd/
//
// Prerequisites:
//   1. docker compose up -d lnd
//   2. Wait for LND to start (~10s)
//   3. ./scripts/copy-lnd-creds.sh
//   4. Ensure lnd-creds/tls.cert and lnd-creds/admin.macaroon exist
// ============================================================================

// projectRoot resolves the project root directory dynamically,
// following the same pattern used in internal/database/test_helper.go.
func projectRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "failed to get caller info")
	// This file is at internal/lnd/client_integration_test.go
	// Project root is 2 directories up.
	return filepath.Join(filepath.Dir(filename), "..", "..")
}

// setupTestLNDClient creates a Client connected to the LND Docker container.
// It skips the test if credentials are not found (LND not set up).
func setupTestLNDClient(t *testing.T) *Client {
	t.Helper()

	root := projectRoot(t)
	certPath := filepath.Join(root, "lnd-creds", "tls.cert")
	macaroonPath := filepath.Join(root, "lnd-creds", "admin.macaroon")

	// Skip gracefully if creds don't exist (LND container not set up)
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		t.Skipf("LND credentials not found at %s — run ./scripts/copy-lnd-creds.sh first", certPath)
	}
	if _, err := os.Stat(macaroonPath); os.IsNotExist(err) {
		t.Skipf("LND macaroon not found at %s — run ./scripts/copy-lnd-creds.sh first", macaroonPath)
	}

	cfg := Config{
		GRPCHost:              "localhost",
		GRPCPort:              "10009",
		TLSCertPath:           certPath,
		MacaroonPath:          macaroonPath,
		Network:               "testnet",
		PaymentTimeoutSeconds: 30,
		MaxPaymentFeeSats:     100,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Skipf("Could not connect to LND (is docker compose up?): %v", err)
	}

	return client
}

// --- NewClient integration test ---

func TestNewClient_ConnectsToLND(t *testing.T) {
	client := setupTestLNDClient(t)
	defer client.Close()

	assert.NotNil(t, client)
	assert.NotNil(t, client.conn)
	assert.NotNil(t, client.lnClient)
	assert.NotNil(t, client.routerClient, "routerClient should be initialized by NewClient")
}

// --- GetInfo ---

func TestClient_GetInfo(t *testing.T) {
	client := setupTestLNDClient(t)
	defer client.Close()

	info, err := client.GetInfo(context.Background())
	require.NoError(t, err)

	assert.NotEmpty(t, info.PubKey, "node should have a pubkey")
	assert.Greater(t, info.BlockHeight, uint32(0), "block height should be > 0")

	t.Logf("LND info: alias=%s pubkey=%s height=%d synced_chain=%t synced_graph=%t",
		info.Alias, info.PubKey, info.BlockHeight, info.SyncedToChain, info.SyncedToGraph)
}

// --- GetWalletBalance ---

func TestClient_GetWalletBalance(t *testing.T) {
	client := setupTestLNDClient(t)
	defer client.Close()

	bal, err := client.GetWalletBalance(context.Background())
	require.NoError(t, err)

	// On a fresh testnet node, balances may be zero — that's fine.
	assert.GreaterOrEqual(t, bal.ConfirmedSats, int64(0))
	assert.GreaterOrEqual(t, bal.TotalSats, int64(0))
	assert.Equal(t, bal.ConfirmedSats+bal.UnconfirmedSats, bal.TotalSats)

	t.Logf("Wallet balance: confirmed=%d unconfirmed=%d total=%d",
		bal.ConfirmedSats, bal.UnconfirmedSats, bal.TotalSats)
}

// --- GetChannelBalance ---

func TestClient_GetChannelBalance(t *testing.T) {
	client := setupTestLNDClient(t)
	defer client.Close()

	bal, err := client.GetChannelBalance(context.Background())
	require.NoError(t, err)

	// A fresh node has no channels, so balances are zero.
	assert.GreaterOrEqual(t, bal.LocalSats, int64(0))
	assert.GreaterOrEqual(t, bal.RemoteSats, int64(0))

	t.Logf("Channel balance: local=%d remote=%d", bal.LocalSats, bal.RemoteSats)
}

// --- NewAddress ---

func TestClient_NewAddress(t *testing.T) {
	client := setupTestLNDClient(t)
	defer client.Close()

	addr, err := client.NewAddress(context.Background())
	require.NoError(t, err)

	assert.NotEmpty(t, addr, "should generate a Bitcoin address")
	// Testnet bech32 addresses start with "tb1"
	assert.Contains(t, addr, "tb1", "testnet bech32 address should start with tb1")

	t.Logf("Generated address: %s", addr)
}

// --- DecodeInvoice ---

func TestClient_DecodeInvoice_InvalidInvoice(t *testing.T) {
	client := setupTestLNDClient(t)
	defer client.Close()

	_, err := client.DecodeInvoice(context.Background(), "lntb_invalid_invoice_string")

	// LND should reject an invalid invoice
	require.Error(t, err)
	t.Logf("Expected error for invalid invoice: %v", err)
}

// --- Close ---

func TestClient_Close(t *testing.T) {
	client := setupTestLNDClient(t)

	err := client.Close()
	assert.NoError(t, err)

	// After closing, gRPC calls should fail
	_, err = client.GetInfo(context.Background())
	assert.Error(t, err, "gRPC call should fail after connection is closed")
}

// --- Multiple connections ---

func TestNewClient_MultipleConcurrentClients(t *testing.T) {
	client1 := setupTestLNDClient(t)
	client2 := setupTestLNDClient(t)
	defer client1.Close()
	defer client2.Close()

	ctx := context.Background()

	// Both clients should be able to call GetInfo concurrently
	info1, err1 := client1.GetInfo(ctx)
	info2, err2 := client2.GetInfo(ctx)

	require.NoError(t, err1)
	require.NoError(t, err2)

	// Same node — same pubkey
	assert.Equal(t, info1.PubKey, info2.PubKey,
		"both clients should connect to the same LND node")
}
