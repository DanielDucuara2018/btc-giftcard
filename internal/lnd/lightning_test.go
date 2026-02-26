package lnd

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/routerrpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// ============================================================================
// Mocks
// ============================================================================

// mockLightningClient implements lnrpc.LightningClient for unit testing.
// Only the methods used by lightning.go are implemented; the rest panic.
type mockLightningClient struct {
	lnrpc.LightningClient // embed to satisfy all interface methods

	decodePayReqFn func(ctx context.Context, in *lnrpc.PayReqString, opts ...grpc.CallOption) (*lnrpc.PayReq, error)
}

func (m *mockLightningClient) DecodePayReq(ctx context.Context, in *lnrpc.PayReqString, opts ...grpc.CallOption) (*lnrpc.PayReq, error) {
	return m.decodePayReqFn(ctx, in, opts...)
}

// mockRouterClient implements routerrpc.RouterClient for unit testing.
type mockRouterClient struct {
	routerrpc.RouterClient

	sendPaymentV2Fn func(ctx context.Context, in *routerrpc.SendPaymentRequest, opts ...grpc.CallOption) (routerrpc.Router_SendPaymentV2Client, error)
}

func (m *mockRouterClient) SendPaymentV2(ctx context.Context, in *routerrpc.SendPaymentRequest, opts ...grpc.CallOption) (routerrpc.Router_SendPaymentV2Client, error) {
	return m.sendPaymentV2Fn(ctx, in, opts...)
}

// mockPaymentStream implements routerrpc.Router_SendPaymentV2Client.
type mockPaymentStream struct {
	grpc.ClientStream
	payments []*lnrpc.Payment
	idx      int
}

func (s *mockPaymentStream) Recv() (*lnrpc.Payment, error) {
	if s.idx >= len(s.payments) {
		return nil, io.EOF
	}
	p := s.payments[s.idx]
	s.idx++
	return p, nil
}

func (s *mockPaymentStream) Header() (metadata.MD, error) { return nil, nil }
func (s *mockPaymentStream) Trailer() metadata.MD         { return nil }
func (s *mockPaymentStream) CloseSend() error             { return nil }
func (s *mockPaymentStream) Context() context.Context     { return context.Background() }
func (s *mockPaymentStream) SendMsg(m interface{}) error  { return nil }
func (s *mockPaymentStream) RecvMsg(m interface{}) error  { return nil }

// newTestClient builds a Client with injected mock dependencies.
func newTestClient(ln lnrpc.LightningClient, router routerrpc.RouterClient) *Client {
	return &Client{
		lnClient:     ln,
		routerClient: router,
		cfg: Config{
			PaymentTimeoutSeconds: 5,
			MaxPaymentFeeSats:     100,
		},
	}
}

// ============================================================================
// DecodeInvoice tests
// ============================================================================

func TestDecodeInvoice_Success(t *testing.T) {
	now := time.Now()
	mock := &mockLightningClient{
		decodePayReqFn: func(_ context.Context, in *lnrpc.PayReqString, _ ...grpc.CallOption) (*lnrpc.PayReq, error) {
			return &lnrpc.PayReq{
				Destination: "03abc",
				NumSatoshis: 50000,
				PaymentHash: "hash123",
				Expiry:      3600,
				Description: "test payment",
				Timestamp:   now.Unix(),
			}, nil
		},
	}

	client := newTestClient(mock, nil)

	invoice, err := client.DecodeInvoice(context.Background(), "lntb500u1...")
	require.NoError(t, err)
	assert.Equal(t, "03abc", invoice.Destination)
	assert.Equal(t, int64(50000), invoice.AmountSats)
	assert.Equal(t, "hash123", invoice.PaymentHash)
	assert.Equal(t, int64(3600), invoice.Expiry)
	assert.Equal(t, "test payment", invoice.Description)
	assert.False(t, invoice.IsExpired, "invoice created now with 1h expiry should not be expired")
}

func TestDecodeInvoice_Expired(t *testing.T) {
	pastTime := time.Now().Add(-2 * time.Hour)
	mock := &mockLightningClient{
		decodePayReqFn: func(_ context.Context, _ *lnrpc.PayReqString, _ ...grpc.CallOption) (*lnrpc.PayReq, error) {
			return &lnrpc.PayReq{
				Destination: "03abc",
				NumSatoshis: 50000,
				PaymentHash: "hash123",
				Expiry:      3600, // 1 hour expiry
				Timestamp:   pastTime.Unix(),
			}, nil
		},
	}

	client := newTestClient(mock, nil)

	invoice, err := client.DecodeInvoice(context.Background(), "lntb500u1...")
	require.NoError(t, err)
	assert.True(t, invoice.IsExpired, "invoice created 2h ago with 1h expiry should be expired")
}

func TestDecodeInvoice_ZeroAmount(t *testing.T) {
	mock := &mockLightningClient{
		decodePayReqFn: func(_ context.Context, _ *lnrpc.PayReqString, _ ...grpc.CallOption) (*lnrpc.PayReq, error) {
			return &lnrpc.PayReq{
				Destination: "03abc",
				NumSatoshis: 0,
				Expiry:      3600,
				Timestamp:   time.Now().Unix(),
			}, nil
		},
	}

	client := newTestClient(mock, nil)

	invoice, err := client.DecodeInvoice(context.Background(), "lntb1...")
	require.NoError(t, err)
	assert.Equal(t, int64(0), invoice.AmountSats)
}

func TestDecodeInvoice_LNDError(t *testing.T) {
	mock := &mockLightningClient{
		decodePayReqFn: func(_ context.Context, _ *lnrpc.PayReqString, _ ...grpc.CallOption) (*lnrpc.PayReq, error) {
			return nil, errors.New("checksum failed")
		},
	}

	client := newTestClient(mock, nil)

	invoice, err := client.DecodeInvoice(context.Background(), "invalid_bolt11")
	assert.Nil(t, invoice)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode invoice")
	assert.Contains(t, err.Error(), "checksum failed")
}

// ============================================================================
// PayInvoice tests
// ============================================================================

func TestPayInvoice_Succeeded(t *testing.T) {
	mockLN := &mockLightningClient{
		decodePayReqFn: func(_ context.Context, _ *lnrpc.PayReqString, _ ...grpc.CallOption) (*lnrpc.PayReq, error) {
			return &lnrpc.PayReq{
				Destination: "03abc",
				NumSatoshis: 50000,
				Expiry:      3600,
				Timestamp:   time.Now().Unix(),
			}, nil
		},
	}

	mockRouter := &mockRouterClient{
		sendPaymentV2Fn: func(_ context.Context, in *routerrpc.SendPaymentRequest, _ ...grpc.CallOption) (routerrpc.Router_SendPaymentV2Client, error) {
			assert.Equal(t, int64(200), in.FeeLimitSat)
			assert.Equal(t, int32(5), in.TimeoutSeconds)

			return &mockPaymentStream{
				payments: []*lnrpc.Payment{
					{Status: lnrpc.Payment_IN_FLIGHT, PaymentHash: "hash1"},
					{
						Status:          lnrpc.Payment_SUCCEEDED,
						PaymentHash:     "hash1",
						PaymentPreimage: "preimage1",
						FeeSat:          5,
					},
				},
			}, nil
		},
	}

	client := newTestClient(mockLN, mockRouter)

	result, err := client.PayInvoice(context.Background(), "lntb500u1...", 200)
	require.NoError(t, err)
	assert.Equal(t, "hash1", result.PaymentHash)
	assert.Equal(t, "preimage1", result.PaymentPreimage)
	assert.Equal(t, int64(5), result.FeeSats)
	assert.Equal(t, suceeded, result.Status)
}

func TestPayInvoice_Failed(t *testing.T) {
	mockLN := &mockLightningClient{
		decodePayReqFn: func(_ context.Context, _ *lnrpc.PayReqString, _ ...grpc.CallOption) (*lnrpc.PayReq, error) {
			return &lnrpc.PayReq{
				NumSatoshis: 50000,
				Expiry:      3600,
				Timestamp:   time.Now().Unix(),
			}, nil
		},
	}

	mockRouter := &mockRouterClient{
		sendPaymentV2Fn: func(_ context.Context, _ *routerrpc.SendPaymentRequest, _ ...grpc.CallOption) (routerrpc.Router_SendPaymentV2Client, error) {
			return &mockPaymentStream{
				payments: []*lnrpc.Payment{
					{
						Status:        lnrpc.Payment_FAILED,
						PaymentHash:   "hash1",
						FailureReason: lnrpc.PaymentFailureReason_FAILURE_REASON_NO_ROUTE,
					},
				},
			}, nil
		},
	}

	client := newTestClient(mockLN, mockRouter)

	result, err := client.PayInvoice(context.Background(), "lntb500u1...", 100)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "payment failed")
	assert.NotNil(t, result)
	assert.Equal(t, failed, result.Status)
	assert.Equal(t, "hash1", result.PaymentHash)
}

func TestPayInvoice_ExpiredInvoice(t *testing.T) {
	pastTime := time.Now().Add(-2 * time.Hour)
	mockLN := &mockLightningClient{
		decodePayReqFn: func(_ context.Context, _ *lnrpc.PayReqString, _ ...grpc.CallOption) (*lnrpc.PayReq, error) {
			return &lnrpc.PayReq{
				NumSatoshis: 50000,
				Expiry:      3600,
				Timestamp:   pastTime.Unix(),
			}, nil
		},
	}

	client := newTestClient(mockLN, nil)

	result, err := client.PayInvoice(context.Background(), "lntb500u1...", 100)
	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invoice is expired")
}

func TestPayInvoice_ZeroAmountInvoice(t *testing.T) {
	mockLN := &mockLightningClient{
		decodePayReqFn: func(_ context.Context, _ *lnrpc.PayReqString, _ ...grpc.CallOption) (*lnrpc.PayReq, error) {
			return &lnrpc.PayReq{
				NumSatoshis: 0,
				Expiry:      3600,
				Timestamp:   time.Now().Unix(),
			}, nil
		},
	}

	client := newTestClient(mockLN, nil)

	result, err := client.PayInvoice(context.Background(), "lntb1...", 100)
	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "zero-amount")
}

func TestPayInvoice_DecodeError(t *testing.T) {
	mockLN := &mockLightningClient{
		decodePayReqFn: func(_ context.Context, _ *lnrpc.PayReqString, _ ...grpc.CallOption) (*lnrpc.PayReq, error) {
			return nil, errors.New("invalid invoice format")
		},
	}

	client := newTestClient(mockLN, nil)

	result, err := client.PayInvoice(context.Background(), "garbage", 100)
	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode invoice")
}

func TestPayInvoice_StreamInitError(t *testing.T) {
	mockLN := &mockLightningClient{
		decodePayReqFn: func(_ context.Context, _ *lnrpc.PayReqString, _ ...grpc.CallOption) (*lnrpc.PayReq, error) {
			return &lnrpc.PayReq{
				NumSatoshis: 50000,
				Expiry:      3600,
				Timestamp:   time.Now().Unix(),
			}, nil
		},
	}

	mockRouter := &mockRouterClient{
		sendPaymentV2Fn: func(_ context.Context, _ *routerrpc.SendPaymentRequest, _ ...grpc.CallOption) (routerrpc.Router_SendPaymentV2Client, error) {
			return nil, errors.New("router unavailable")
		},
	}

	client := newTestClient(mockLN, mockRouter)

	result, err := client.PayInvoice(context.Background(), "lntb500u1...", 100)
	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initiate payment")
}

func TestPayInvoice_StreamRecvError(t *testing.T) {
	mockLN := &mockLightningClient{
		decodePayReqFn: func(_ context.Context, _ *lnrpc.PayReqString, _ ...grpc.CallOption) (*lnrpc.PayReq, error) {
			return &lnrpc.PayReq{
				NumSatoshis: 50000,
				Expiry:      3600,
				Timestamp:   time.Now().Unix(),
			}, nil
		},
	}

	mockRouter := &mockRouterClient{
		sendPaymentV2Fn: func(_ context.Context, _ *routerrpc.SendPaymentRequest, _ ...grpc.CallOption) (routerrpc.Router_SendPaymentV2Client, error) {
			return &mockPaymentStream{
				payments: nil, // empty — Recv returns io.EOF immediately
			}, nil
		},
	}

	client := newTestClient(mockLN, mockRouter)

	result, err := client.PayInvoice(context.Background(), "lntb500u1...", 100)
	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "payment stream error")
}

func TestPayInvoice_InFlightThenSucceeded(t *testing.T) {
	mockLN := &mockLightningClient{
		decodePayReqFn: func(_ context.Context, _ *lnrpc.PayReqString, _ ...grpc.CallOption) (*lnrpc.PayReq, error) {
			return &lnrpc.PayReq{
				NumSatoshis: 1000,
				Expiry:      3600,
				Timestamp:   time.Now().Unix(),
			}, nil
		},
	}

	mockRouter := &mockRouterClient{
		sendPaymentV2Fn: func(_ context.Context, _ *routerrpc.SendPaymentRequest, _ ...grpc.CallOption) (routerrpc.Router_SendPaymentV2Client, error) {
			return &mockPaymentStream{
				payments: []*lnrpc.Payment{
					{Status: lnrpc.Payment_INITIATED, PaymentHash: "h1"},
					{Status: lnrpc.Payment_IN_FLIGHT, PaymentHash: "h1"},
					{Status: lnrpc.Payment_IN_FLIGHT, PaymentHash: "h1"},
					{
						Status:          lnrpc.Payment_SUCCEEDED,
						PaymentHash:     "h1",
						PaymentPreimage: "pre1",
						FeeSat:          2,
					},
				},
			}, nil
		},
	}

	client := newTestClient(mockLN, mockRouter)

	result, err := client.PayInvoice(context.Background(), "lntb10u1...", 50)
	require.NoError(t, err)
	assert.Equal(t, suceeded, result.Status)
	assert.Equal(t, "pre1", result.PaymentPreimage)
	assert.Equal(t, int64(2), result.FeeSats)
}

func TestPayInvoice_RequestFieldsPassedCorrectly(t *testing.T) {
	var capturedReq *routerrpc.SendPaymentRequest

	mockLN := &mockLightningClient{
		decodePayReqFn: func(_ context.Context, _ *lnrpc.PayReqString, _ ...grpc.CallOption) (*lnrpc.PayReq, error) {
			return &lnrpc.PayReq{
				NumSatoshis: 10000,
				Expiry:      3600,
				Timestamp:   time.Now().Unix(),
			}, nil
		},
	}

	mockRouter := &mockRouterClient{
		sendPaymentV2Fn: func(_ context.Context, in *routerrpc.SendPaymentRequest, _ ...grpc.CallOption) (routerrpc.Router_SendPaymentV2Client, error) {
			capturedReq = in
			return &mockPaymentStream{
				payments: []*lnrpc.Payment{
					{Status: lnrpc.Payment_SUCCEEDED, PaymentHash: "h1", PaymentPreimage: "p1"},
				},
			}, nil
		},
	}

	client := newTestClient(mockLN, mockRouter)
	client.cfg.PaymentTimeoutSeconds = 45

	_, err := client.PayInvoice(context.Background(), "lntb100u1bolt11here", 250)
	require.NoError(t, err)

	require.NotNil(t, capturedReq)
	assert.Equal(t, "lntb100u1bolt11here", capturedReq.PaymentRequest)
	assert.Equal(t, int32(45), capturedReq.TimeoutSeconds)
	assert.Equal(t, int64(250), capturedReq.FeeLimitSat)
}
