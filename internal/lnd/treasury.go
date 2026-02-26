package lnd

import (
	"context"
	"fmt"

	"github.com/lightningnetwork/lnd/lnrpc"
)

// GetChannelBalance returns the balance across all open Lightning channels.
//   - LocalSats:  our side — sats we can send via Lightning right now
//   - RemoteSats: their side — sats we can receive via Lightning right now
//
// LocalSats represents the liquidity locked in Lightning channels that
// backs outstanding card balances redeemable via Lightning.
func (c *Client) GetChannelBalance(ctx context.Context) (*ChannelBalance, error) {
	resp, err := c.lnClient.ChannelBalance(ctx, &lnrpc.ChannelBalanceRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get channel balance: %w", err)
	}

	var localSats, remoteSats int64
	if resp.LocalBalance != nil {
		localSats = int64(resp.LocalBalance.Sat)
	}
	if resp.RemoteBalance != nil {
		remoteSats = int64(resp.RemoteBalance.Sat)
	}

	return &ChannelBalance{
		LocalSats:  localSats,
		RemoteSats: remoteSats,
	}, nil
}

// GetInfo returns basic LND node information.
// Used at startup (NewClient) for health validation and by the /health endpoint.
func (c *Client) GetInfo(ctx context.Context) (*NodeInfo, error) {
	resp, err := c.lnClient.GetInfo(ctx, &lnrpc.GetInfoRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get node info: %w", err)
	}

	return &NodeInfo{
		Alias:         resp.Alias,
		PubKey:        resp.IdentityPubkey,
		SyncedToChain: resp.SyncedToChain,
		SyncedToGraph: resp.SyncedToGraph,
		BlockHeight:   resp.BlockHeight,
		NumChannels:   resp.NumActiveChannels,
	}, nil
}
