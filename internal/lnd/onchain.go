package lnd

import (
	"context"
	"errors"
	"fmt"

	"github.com/lightningnetwork/lnd/lnrpc"
)

// SendOnChain sends BTC from LND's on-chain wallet to a destination address.
// targetConf controls fee estimation: 2=next block, 6=~1h (default), 144=~1day.
func (c *Client) SendOnChain(ctx context.Context, address string, amountSats int64, targetConf int32) (*OnChainResult, error) {
	if address == "" {
		return nil, errors.New("address must not be empty")
	}

	// Bitcoin dust limit: outputs below 546 sats are rejected by the network.
	if amountSats < 546 {
		return nil, fmt.Errorf("amount %d is below dust limit (546 sats)", amountSats)
	}

	req := &lnrpc.SendCoinsRequest{
		Addr:       address,
		Amount:     amountSats,
		TargetConf: targetConf,
	}

	resp, err := c.lnClient.SendCoins(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to send on-chain coins: %w", err)
	}

	return &OnChainResult{TxHash: resp.Txid}, nil
}

// NewAddress generates a new native SegWit (bech32) deposit address from
// LND's HD wallet. Each call derives a fresh address.
func (c *Client) NewAddress(ctx context.Context) (string, error) {
	req := &lnrpc.NewAddressRequest{
		Type: lnrpc.AddressType_WITNESS_PUBKEY_HASH, // bech32 bc1q... — lowest fees
	}

	resp, err := c.lnClient.NewAddress(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to generate new address: %w", err)
	}

	return resp.Address, nil
}

// GetWalletBalance returns LND's on-chain wallet balance split into confirmed
// and unconfirmed amounts. Used by the treasury service to assess spendable funds.
func (c *Client) GetWalletBalance(ctx context.Context) (*WalletBalance, error) {
	resp, err := c.lnClient.WalletBalance(ctx, &lnrpc.WalletBalanceRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet balance: %w", err)
	}

	return &WalletBalance{
		ConfirmedSats:   resp.ConfirmedBalance,
		UnconfirmedSats: resp.UnconfirmedBalance,
		TotalSats:       resp.TotalBalance,
	}, nil
}
