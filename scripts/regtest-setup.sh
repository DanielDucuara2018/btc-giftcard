#!/usr/bin/env bash
# =============================================================================
# regtest-setup.sh — Bootstrap the full regtest environment in one command.
#
# What this script does:
#   1. Tears down any existing stack + volumes (fresh start, unless --no-reset)
#   2. Starts bitcoind only and mines 1 IBD block before LND starts
#      (prevents the "-8: Block height out of range" wallet-sync error)
#   3. Starts all remaining services (LND nodes, app, worker, etc.)
#   4. Waits for the API and operator LND to be ready
#   5. Mines 101 blocks → operator wallet (funds + matures coinbase)
#   6. Connects the two LND nodes as peers
#   7. Opens a channel: operator → customer
#   8. Mines 6 blocks to activate the channel
#   9. Prints a summary and a ready-to-use curl example
#
# Usage:
#   ./scripts/regtest-setup.sh             # fresh start (wipes volumes — recommended)
#   ./scripts/regtest-setup.sh --no-reset  # resume existing stack without wiping
#
# The --no-reset flag is for advanced use only.  All three regtest data volumes
# (bitcoind_data_regtest, lnd_data_regtest, lnd_data_regtest_customer) must stay
# in sync.  If bitcoind is reset independently, LND's stored wallet birthday will
# be higher than the chain height → "Block height out of range" error loop.
#
# Prerequisites: docker compose v2, curl, jq
#
# Docker access:
#   Locally, the current user is expected to have access to the Docker socket.
#   On the testing VM (GCE + IAP SSH), the IAP session user does not belong to
#   the docker group.  The script detects this and re-executes itself as the
#   'gifter' OS user (who was added to the docker group by startup.sh) using
#   sudo.  No manual user-switching is required.
# =============================================================================

set -euo pipefail

# ---------------------------------------------------------------------------
# Docker access check — re-exec as 'gifter' on the VM if needed.
#
# We check before set -euo pipefail takes effect so the test can fail silently.
# The REGTEST_REEXEC guard prevents infinite re-exec loops.
# ---------------------------------------------------------------------------
if [[ "${REGTEST_REEXEC:-}" != "1" ]]; then
  if ! docker info > /dev/null 2>&1; then
    if id -u gifter > /dev/null 2>&1; then
      echo "[regtest-setup] Current user cannot reach Docker; re-executing as 'gifter' ..."
      exec sudo -u gifter env REGTEST_REEXEC=1 bash "$0" "$@"
    else
      echo "[regtest-setup] ERROR: Cannot connect to the Docker socket." >&2
      echo "  Ensure your user is in the 'docker' group or run with sudo." >&2
      exit 1
    fi
  fi
fi

COMPOSE="docker compose -f docker-compose.yml -f docker-compose.regtest.yml"
API="http://localhost:3202"
CHANNEL_SIZE_SATS=500000   # outbound capacity for the operator → customer channel
RESET=true

# ---------------------------------------------------------------------------
# Parse flags
# ---------------------------------------------------------------------------
for arg in "$@"; do
  case "$arg" in
    --no-reset) RESET=false ;;
    *) echo "[regtest-setup] Unknown argument: $arg" >&2; exit 1 ;;
  esac
done

log()  { echo "[regtest-setup] $*"; }
ok()   { echo "[regtest-setup] ✓ $*"; }
fail() { echo "[regtest-setup] ✗ $*" >&2; exit 1; }

# ---------------------------------------------------------------------------
# 1. Optionally tear down the existing stack and wipe all regtest volumes.
#
# WHY this is the default:
#   bitcoind, lnd, and lnd_customer each store chain/wallet state in named
#   Docker volumes.  If any one of them is reset (e.g. container recreated)
#   without resetting the others, the wallet birthday stored in the LND volume
#   will be greater than the chain tip in bitcoind → LND loops with:
#     [ERR] LNWL: Unable to synchronize wallet to chain, trying again in 5s:
#           -8: Block height out of range
#   Wiping all three volumes together on every setup run guarantees they are
#   always in sync from block 0.
# ---------------------------------------------------------------------------
if [[ "$RESET" == "true" ]]; then
  log "Tearing down existing regtest stack and wiping volumes (use --no-reset to skip)..."
  $COMPOSE down -v --remove-orphans 2>/dev/null || true
  ok "Existing stack torn down, volumes wiped"
fi

# ---------------------------------------------------------------------------
# 2. Start bitcoind first and mine 1 IBD block BEFORE any LND node starts.
#
# WHY staged startup:
#   Bitcoin Core reports initialblockdownload=true whenever the chain tip is
#   older than 24 hours.  On a brand-new regtest chain the genesis block is
#   dated January 2009, so bitcoind stays in IBD indefinitely.  LND will
#   never set synced_to_chain=true while its backend is in IBD.
#
#   Mining 1 block advances the chain tip to "now" and exits IBD.  We do this
#   BEFORE starting LND so that LND's wallet is created at block height ≥ 1,
#   and the first wallet-sync attempt never requests a block that doesn't exist.
# ---------------------------------------------------------------------------
log "Starting bitcoind..."
$COMPOSE up -d bitcoind

log "Waiting for bitcoind to be ready..."
for i in $(seq 1 30); do
  $COMPOSE exec -T bitcoind bitcoin-cli -regtest -rpcuser=btc -rpcpassword=btc \
    getblockchaininfo > /dev/null 2>&1 && break
  [[ $i -eq 30 ]] && fail "bitcoind did not become ready within 60s"
  sleep 2
done
ok "bitcoind ready"

log "Mining 1 block to exit IBD mode (genesis block is from 2009)..."
# Bitcoin Core v22+ no longer creates a default wallet automatically.
$COMPOSE exec -T bitcoind bitcoin-cli -regtest -rpcuser=btc -rpcpassword=btc \
  createwallet "default" > /dev/null 2>&1 || true
IBD_ADDR=$($COMPOSE exec -T bitcoind bitcoin-cli -regtest -rpcuser=btc -rpcpassword=btc \
  getnewaddress 2>/dev/null | tr -d '[:space:]')
$COMPOSE exec -T bitcoind bitcoin-cli -regtest -rpcuser=btc -rpcpassword=btc \
  generatetoaddress 1 "$IBD_ADDR" > /dev/null
ok "IBD block mined (height=1) — bitcoind exited IBD mode"

# ---------------------------------------------------------------------------
# 3. Start all remaining services
#
# LND depends_on bitcoind:service_healthy (already healthy above), and now
# bitcoind is at height 1 instead of 0.  LND will create its wallet with
# birthday=1, ensuring the first wallet-sync call succeeds.
# ---------------------------------------------------------------------------
log "Starting remaining services (LND nodes, app, worker, etc.)..."
$COMPOSE up -d

# ---------------------------------------------------------------------------
# 4. Wait for the API to be healthy
# ---------------------------------------------------------------------------
log "Waiting for API server to be ready..."
for i in $(seq 1 600); do
  if curl -sf "${API}/health" > /dev/null 2>&1; then
    ok "API server is up"
    break
  fi
  [[ $i -eq 600 ]] && fail "API server did not start within 600s"
  sleep 2
done

# ---------------------------------------------------------------------------
# 5. Wait for operator LND to sync
# ---------------------------------------------------------------------------
log "Waiting for operator LND to sync..."
for i in $(seq 1 60); do
  SYNCED=$(curl -sf "${API}/api/node/info" 2>/dev/null | jq -r '.synced_to_chain // false')
  if [[ "$SYNCED" == "true" ]]; then
    ok "Operator LND synced"
    break
  fi
  [[ $i -eq 60 ]] && fail "Operator LND did not sync within 60s — check: docker compose logs lnd"
  sleep 2
done

# ---------------------------------------------------------------------------
# 6. Mine 101 blocks to the operator wallet
# (101 = 100 coinbase maturity blocks + 1 extra)
# ---------------------------------------------------------------------------
log "Getting operator deposit address..."
OP_ADDR=$(curl -sf -X POST "${API}/api/node/wallet/address" | jq -r '.address')
[[ -z "$OP_ADDR" ]] && fail "Could not get operator wallet address"
log "Mining 101 blocks to operator: ${OP_ADDR}"
$COMPOSE exec -T bitcoind bitcoin-cli -regtest -rpcuser=btc -rpcpassword=btc \
  generatetoaddress 101 "$OP_ADDR" > /dev/null
ok "Mined 101 blocks — operator wallet funded"

# Short pause for LND to index the blocks
sleep 3

# ---------------------------------------------------------------------------
# 7. Get customer LND pubkey and connect peers
# ---------------------------------------------------------------------------
log "Getting customer LND identity pubkey..."
CUSTOMER_PUBKEY=$($COMPOSE exec -T lnd_customer \
  lncli --network=regtest getinfo 2>/dev/null | jq -r '.identity_pubkey')
[[ -z "$CUSTOMER_PUBKEY" ]] && fail "Could not get customer LND pubkey"
ok "Customer pubkey: ${CUSTOMER_PUBKEY}"

log "Connecting operator → customer as peers..."
curl -sf -X POST "${API}/api/node/peers" \
  -H "Content-Type: application/json" \
  -d "{\"pub_key\":\"${CUSTOMER_PUBKEY}\",\"host\":\"gift-card-backend.lnd-customer:9735\"}" \
  > /dev/null || true   # ignore "already connected" errors
ok "Peers connected"

# ---------------------------------------------------------------------------
# 8. Open a channel operator → customer
# ---------------------------------------------------------------------------
log "Opening channel (operator → customer, ${CHANNEL_SIZE_SATS} sats)..."
CHAN_RESULT=$(curl -sf -X POST "${API}/api/node/channels" \
  -H "Content-Type: application/json" \
  -d "{\"peer_pub_key\":\"${CUSTOMER_PUBKEY}\",\"local_amt_sats\":${CHANNEL_SIZE_SATS},\"push_amt_sats\":0,\"target_conf\":1}")
FUNDING_TXID=$(echo "$CHAN_RESULT" | jq -r '.funding_txid')
ok "Channel funding tx: ${FUNDING_TXID}"

# ---------------------------------------------------------------------------
# 9. Mine 6 blocks to confirm the channel
# ---------------------------------------------------------------------------
log "Mining 6 blocks to activate channel..."
$COMPOSE exec -T bitcoind bitcoin-cli -regtest -rpcuser=btc -rpcpassword=btc \
  generatetoaddress 6 "$OP_ADDR" > /dev/null

# ---------------------------------------------------------------------------
# 10. Summary
# ---------------------------------------------------------------------------
WALLET_BALANCE=$(curl -sf "${API}/api/node/wallet/balance" | jq '.')
CHANNEL_BALANCE=$(curl -sf "${API}/api/node/channels/balance" | jq '.')

echo ""
echo "============================================================"
echo "  Regtest environment ready"
echo "============================================================"
echo ""
echo "  Operator wallet balance:"
echo "$WALLET_BALANCE" | jq '.'
echo ""
echo "  Operator channel balance:"
echo "$CHANNEL_BALANCE" | jq '.'
echo ""
echo "  API:  ${API}"
echo ""
echo "  --- Example: full gift card lifecycle ---"
echo ""
echo "  # 1. Create a card"
echo "  curl -X POST ${API}/api/cards -H 'Content-Type: application/json' \\"
echo "    -d '{\"fiat_amount_cents\":1000,\"fiat_currency\":\"EUR\",\"purchase_email\":\"test@example.com\"}' | jq ."
echo ""
echo "  # 2. Generate a customer invoice (replace <SATS> with the card's btc_amount_sats)"
echo "  ${COMPOSE} exec lnd_customer lncli --network=regtest addinvoice --amt <SATS>"
echo ""
echo "  # 3. Redeem the card"
echo "  curl -X POST ${API}/api/cards/<CODE>/redeem -H 'Content-Type: application/json' \\"
echo "    -d '{\"method\":\"lightning\",\"invoice\":\"<BOLT11>\"}' | jq ."
echo ""
echo "  # Mine blocks for on-chain confirmations at any time:"
echo "  ${COMPOSE} exec -T bitcoind bitcoin-cli -regtest -rpcuser=btc -rpcpassword=btc generatetoaddress 6 ${OP_ADDR}"
echo "============================================================"
