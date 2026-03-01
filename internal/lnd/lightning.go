package lnd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/routerrpc"
)

// PayInvoice pays a BOLT11 invoice using the Router sub-server's SendPaymentV2
// streaming RPC. It validates the invoice first, then sends the payment and
// waits for a terminal state (SUCCEEDED or FAILED).
func (c *Client) PayInvoice(ctx context.Context, bolt11 string, maxFeeSats int64) (*PaymentResult, error) {
	invoice, err := c.DecodeInvoice(ctx, bolt11)
	if err != nil {
		return nil, fmt.Errorf("failed to decode invoice: %w", err)
	}

	if invoice.IsExpired {
		return nil, errors.New("invoice is expired")
	}

	if invoice.AmountSats == 0 {
		return nil, errors.New("zero-amount invoices are not supported")
	}

	req := &routerrpc.SendPaymentRequest{
		PaymentRequest: bolt11,
		TimeoutSeconds: int32(c.Cfg.PaymentTimeoutSeconds),
		FeeLimitSat:    maxFeeSats,
	}

	payCtx, cancel := context.WithTimeout(ctx, time.Duration(c.Cfg.PaymentTimeoutSeconds)*time.Second)
	defer cancel()

	stream, err := c.routerClient.SendPaymentV2(payCtx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to initiate payment: %w", err)
	}

	// Read payment status updates from the stream until we reach a terminal state.
	for {
		payment, err := stream.Recv()
		if err != nil {
			return nil, fmt.Errorf("payment stream error: %w", err)
		}

		switch payment.Status {
		case lnrpc.Payment_SUCCEEDED:
			return &PaymentResult{
				PaymentHash:     payment.PaymentHash,
				PaymentPreimage: payment.PaymentPreimage,
				FeeSats:         payment.FeeSat,
				Status:          Succeeded,
			}, nil

		case lnrpc.Payment_FAILED:
			return &PaymentResult{
				PaymentHash: payment.PaymentHash,
				Status:      Failed,
			}, fmt.Errorf("payment failed: %s", payment.FailureReason)

		case lnrpc.Payment_IN_FLIGHT, lnrpc.Payment_INITIATED:
			// Payment still in progress, continue reading the stream.
			continue

		default:
			return nil, fmt.Errorf("unexpected payment status: %s", payment.Status)
		}
	}
}

// DecodeInvoice decodes a BOLT11 invoice string without paying it.
// Used to validate invoice amount, expiry, and network before payment.
func (c *Client) DecodeInvoice(ctx context.Context, bolt11 string) (*Invoice, error) {
	resp, err := c.lnClient.DecodePayReq(ctx, &lnrpc.PayReqString{PayReq: bolt11})
	if err != nil {
		return nil, fmt.Errorf("failed to decode invoice: %w", err)
	}

	expiryTime := time.Unix(resp.Timestamp+resp.Expiry, 0)
	isExpired := time.Now().After(expiryTime)

	return &Invoice{
		Destination: resp.Destination,
		AmountSats:  resp.NumSatoshis,
		PaymentHash: resp.PaymentHash,
		Expiry:      resp.Expiry,
		Description: resp.Description,
		IsExpired:   isExpired,
	}, nil
}
