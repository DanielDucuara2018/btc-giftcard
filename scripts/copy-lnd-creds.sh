#!/usr/bin/env bash
# Copy LND TLS cert and macaroon from the Docker named volume to the host.
# Run this once after first `docker compose up -d lnd`.

set -euo pipefail

DEST_DIR="./lnd-creds"
NETWORK="${1:-testnet}"

echo "Copying LND credentials (network: ${NETWORK})..."

mkdir -p "${DEST_DIR}"

docker compose cp "lnd:/root/.lnd/tls.cert" "${DEST_DIR}/tls.cert"
docker compose cp "lnd:/root/.lnd/data/chain/bitcoin/${NETWORK}/admin.macaroon" "${DEST_DIR}/admin.macaroon"

echo "Done. Files saved to ${DEST_DIR}/:"
ls -la "${DEST_DIR}/"
echo ""
echo "Update config.toml:"
echo '  tls_cert_path = "./lnd-creds/tls.cert"'
echo '  macaroon_path = "./lnd-creds/admin.macaroon"'
