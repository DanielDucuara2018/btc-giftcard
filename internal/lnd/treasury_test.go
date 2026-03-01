package lnd

import (
	"context"
	"errors"
	"testing"

	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// ============================================================================
// Mock — stubs the lnrpc.LightningClient methods used by treasury.go
// ============================================================================

type mockTreasuryLNClient struct {
	lnrpc.LightningClient // embed for interface compliance

	channelBalanceFn func(ctx context.Context, in *lnrpc.ChannelBalanceRequest, opts ...grpc.CallOption) (*lnrpc.ChannelBalanceResponse, error)
	getInfoFn        func(ctx context.Context, in *lnrpc.GetInfoRequest, opts ...grpc.CallOption) (*lnrpc.GetInfoResponse, error)
}

func (m *mockTreasuryLNClient) ChannelBalance(ctx context.Context, in *lnrpc.ChannelBalanceRequest, opts ...grpc.CallOption) (*lnrpc.ChannelBalanceResponse, error) {
	return m.channelBalanceFn(ctx, in, opts...)
}

func (m *mockTreasuryLNClient) GetInfo(ctx context.Context, in *lnrpc.GetInfoRequest, opts ...grpc.CallOption) (*lnrpc.GetInfoResponse, error) {
	return m.getInfoFn(ctx, in, opts...)
}

func newTreasuryTestClient(mock *mockTreasuryLNClient) *Client {
	return &Client{
		lnClient: mock,
		Cfg:      Config{},
	}
}

// ============================================================================
// GetChannelBalance tests
// ============================================================================

func TestGetChannelBalance_Success(t *testing.T) {
	mock := &mockTreasuryLNClient{
		channelBalanceFn: func(_ context.Context, _ *lnrpc.ChannelBalanceRequest, _ ...grpc.CallOption) (*lnrpc.ChannelBalanceResponse, error) {
			return &lnrpc.ChannelBalanceResponse{
				LocalBalance:  &lnrpc.Amount{Sat: 500000, Msat: 500000000},
				RemoteBalance: &lnrpc.Amount{Sat: 300000, Msat: 300000000},
			}, nil
		},
	}

	client := newTreasuryTestClient(mock)
	bal, err := client.GetChannelBalance(context.Background())

	require.NoError(t, err)
	assert.Equal(t, int64(500000), bal.LocalSats)
	assert.Equal(t, int64(300000), bal.RemoteSats)
}

func TestGetChannelBalance_NilBalances(t *testing.T) {
	// A fresh node with no channels returns nil LocalBalance/RemoteBalance.
	mock := &mockTreasuryLNClient{
		channelBalanceFn: func(_ context.Context, _ *lnrpc.ChannelBalanceRequest, _ ...grpc.CallOption) (*lnrpc.ChannelBalanceResponse, error) {
			return &lnrpc.ChannelBalanceResponse{
				LocalBalance:  nil,
				RemoteBalance: nil,
			}, nil
		},
	}

	client := newTreasuryTestClient(mock)
	bal, err := client.GetChannelBalance(context.Background())

	require.NoError(t, err)
	assert.Equal(t, int64(0), bal.LocalSats, "nil LocalBalance should map to 0")
	assert.Equal(t, int64(0), bal.RemoteSats, "nil RemoteBalance should map to 0")
}

func TestGetChannelBalance_OnlyLocalBalance(t *testing.T) {
	mock := &mockTreasuryLNClient{
		channelBalanceFn: func(_ context.Context, _ *lnrpc.ChannelBalanceRequest, _ ...grpc.CallOption) (*lnrpc.ChannelBalanceResponse, error) {
			return &lnrpc.ChannelBalanceResponse{
				LocalBalance:  &lnrpc.Amount{Sat: 100000},
				RemoteBalance: nil,
			}, nil
		},
	}

	client := newTreasuryTestClient(mock)
	bal, err := client.GetChannelBalance(context.Background())

	require.NoError(t, err)
	assert.Equal(t, int64(100000), bal.LocalSats)
	assert.Equal(t, int64(0), bal.RemoteSats)
}

func TestGetChannelBalance_LNDError(t *testing.T) {
	mock := &mockTreasuryLNClient{
		channelBalanceFn: func(_ context.Context, _ *lnrpc.ChannelBalanceRequest, _ ...grpc.CallOption) (*lnrpc.ChannelBalanceResponse, error) {
			return nil, errors.New("connection refused")
		},
	}

	client := newTreasuryTestClient(mock)
	bal, err := client.GetChannelBalance(context.Background())

	assert.Nil(t, bal)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get channel balance")
	assert.Contains(t, err.Error(), "connection refused")
}

// ============================================================================
// GetInfo tests
// ============================================================================

func TestGetInfo_Success(t *testing.T) {
	mock := &mockTreasuryLNClient{
		getInfoFn: func(_ context.Context, _ *lnrpc.GetInfoRequest, _ ...grpc.CallOption) (*lnrpc.GetInfoResponse, error) {
			return &lnrpc.GetInfoResponse{
				Alias:             "btc-giftcard-node",
				IdentityPubkey:    "03abc123def456",
				SyncedToChain:     true,
				SyncedToGraph:     true,
				BlockHeight:       850000,
				NumActiveChannels: 5,
			}, nil
		},
	}

	client := newTreasuryTestClient(mock)
	info, err := client.GetInfo(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "btc-giftcard-node", info.Alias)
	assert.Equal(t, "03abc123def456", info.PubKey)
	assert.True(t, info.SyncedToChain)
	assert.True(t, info.SyncedToGraph)
	assert.Equal(t, uint32(850000), info.BlockHeight)
	assert.Equal(t, uint32(5), info.NumChannels)
}

func TestGetInfo_NotSynced(t *testing.T) {
	mock := &mockTreasuryLNClient{
		getInfoFn: func(_ context.Context, _ *lnrpc.GetInfoRequest, _ ...grpc.CallOption) (*lnrpc.GetInfoResponse, error) {
			return &lnrpc.GetInfoResponse{
				Alias:             "syncing-node",
				IdentityPubkey:    "03xyz",
				SyncedToChain:     false,
				SyncedToGraph:     false,
				BlockHeight:       100,
				NumActiveChannels: 0,
			}, nil
		},
	}

	client := newTreasuryTestClient(mock)
	info, err := client.GetInfo(context.Background())

	require.NoError(t, err)
	assert.False(t, info.SyncedToChain)
	assert.False(t, info.SyncedToGraph)
	assert.Equal(t, uint32(0), info.NumChannels)
}

func TestGetInfo_LNDError(t *testing.T) {
	mock := &mockTreasuryLNClient{
		getInfoFn: func(_ context.Context, _ *lnrpc.GetInfoRequest, _ ...grpc.CallOption) (*lnrpc.GetInfoResponse, error) {
			return nil, errors.New("wallet locked")
		},
	}

	client := newTreasuryTestClient(mock)
	info, err := client.GetInfo(context.Background())

	assert.Nil(t, info)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get node info")
	assert.Contains(t, err.Error(), "wallet locked")
}
