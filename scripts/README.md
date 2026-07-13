# btc-giftcard — scripts

Scripts which live here:

| Script | Purpose |
|--------|---------|
| [`setup-vm.sh`](#setup-vmsh) | Deploy the stack to a GCP testing VM |
| [`regtest-setup.sh`](#regtest-setupsh) | Bootstrap a local regtest environment from scratch |

---

## setup-vm.sh

Deploys the `docker-compose.regtest.yml` stack to a GCP GCE VM provisioned by
[btc-giftcard-infrastructure](https://github.com/DanielDucuara2018/btc-giftcard-infrastructure).

### How it resolves the VM

The script reads VM connection details from Terraform outputs automatically.
It looks for the infra repo in two places (relative to its own location):

1. `../../btc-giftcard-infrastructure/envs/testing` — standard side-by-side layout
2. `../envs/testing` — if the script were inside the infra repo itself

If neither is found, pass `--infra-dir` explicitly, or supply all four VM flags
directly (`--vm-name`, `--vm-zone`, `--project`, `--subdomain`).

### Prerequisites

- `gcloud` CLI authenticated: `gcloud auth login`
- `terraform` on `$PATH` (only needed when auto-detecting VM details)
- `terraform apply` already run in `btc-giftcard-infrastructure/envs/testing/`
- Your Google account has `roles/iap.tunnelResourceAccessor` on the GCP project
- Cloudflare A record for the API subdomain pointing to the VM IP (**DNS only**, not proxied)

### Modes

#### `--init` — First-time setup

Run once after `terraform apply`. Performs in sequence:

1. Waits for the VM startup script to finish (installs docker, nginx, certbot)
2. Prompts you to confirm DNS is pointing to the VM
3. Deploys application code (git clone **or** local sync — see below)
4. Uploads the `.env` file to the VM
5. Configures Nginx as a reverse proxy for port 3202
6. Requests a Let's Encrypt TLS certificate via certbot
7. Starts the full `docker-compose.regtest.yml` stack

```bash
# Git-based (default) — deploys the latest committed code
./scripts/setup-vm.sh --init \
  --email admin@gifter.danobhub.com \
  --repo https://github.com/you/btc-giftcard \
  --env .env

# Local-sync — deploy your working directory (uncommitted changes)
./scripts/setup-vm.sh --init \
  --local . \
  --email admin@gifter.danobhub.com \
  --env .env
```

Required for `--init`: `--email`

#### `--deploy` — Update and restart

Pulls the latest code (or re-syncs local files) and restarts the stack.

```bash
# Pull latest committed code
./scripts/setup-vm.sh --deploy

# Also update .env
./scripts/setup-vm.sh --deploy --env .env

# Push local working directory (dev iteration)
./scripts/setup-vm.sh --deploy --local .
```

#### `--status` — Health check

Prints `docker compose ps` output and calls `GET /health` on the API.

```bash
./scripts/setup-vm.sh --status
```

### Source modes

#### Git-based (`--repo URL` or default pull)

- On `--init`: clones the repo into `/opt/gifter/btc-giftcard` on the VM
- On `--deploy`: runs `git pull` in the existing clone
- Reproducible — every deploy corresponds to a commit SHA
- Recommended for shared testing and production

#### Local sync (`--local PATH`)

- Packs the local directory into a `tar` archive and streams it over the IAP
  SSH connection; no rsync daemon or extra port needed
- Excludes: `.git`, `.env`, `lnd-creds/`, `tmp/`, `*.log`, `*.swp`
- `.env` is always uploaded separately via `--env` so it is never included in
  the sync archive
- **Does not delete** files present on the VM that were removed locally. For a
  clean slate, SSH in and remove the directory first:
  ```bash
  gcloud compute ssh btcgifter-app-vm --zone=europe-west4-a --project=hispanie \
    --tunnel-through-iap -- "sudo rm -rf /opt/gifter/btc-giftcard"
  ```
- **WARNING: deploys uncommitted changes — development and testing only.
  Never use `--local` against a production environment.**

### All flags

| Flag | Description |
|------|-------------|
| `--init` | First-time VM setup |
| `--deploy` | Update code and restart services |
| `--status` | Show docker ps + API health |
| `--repo URL` | Git clone/pull source (default mode) |
| `--local PATH` | Local directory sync source |
| `--email EMAIL` | Let's Encrypt contact email (required for `--init`) |
| `--env PATH` | Local `.env` file to upload to the VM |
| `--infra-dir PATH` | Override auto-detected infra repo location |
| `--vm-name NAME` | GCE instance name (skips Terraform lookup) |
| `--vm-zone ZONE` | GCE zone (skips Terraform lookup) |
| `--project ID` | GCP project ID (skips Terraform lookup) |
| `--subdomain HOST` | API hostname, e.g. `api.gifter.danobhub.com` |
| `--vm-ip IP` | VM external IP (used only for DNS confirmation prompt) |

### Typical workflow

```bash
# 1. Provision infra (once)
cd ../btc-giftcard-infrastructure/envs/testing
terraform apply

# 2. Update Cloudflare A record to the VM IP
terraform output app_vm_ip

# 3. First-time init (from the backend repo root)
cd /path/to/btc-giftcard
./scripts/setup-vm.sh --init --email admin@danobhub.com --repo <REPO_URL> --env .env

# 4. SSH in and run regtest bootstrap
gcloud compute ssh btcgifter-app-vm --zone=europe-west4-a --project=hispanie \
  --tunnel-through-iap -- "cd /opt/gifter/btc-giftcard && ./scripts/regtest-setup.sh"

# 5. Iterate on code locally, then push changes
./scripts/setup-vm.sh --deploy --local .

# 6. Check everything is healthy
./scripts/setup-vm.sh --status
```

### Troubleshooting

**VM not ready / startup script timed out**

Check the serial console for startup errors:
```bash
gcloud compute instances get-serial-port-output btcgifter-app-vm \
  --zone=europe-west4-a --project=hispanie
```

**certbot fails**

Certbot cannot obtain a certificate if DNS is not yet pointing to the VM, or if
the Cloudflare proxy is enabled (orange cloud). Confirm:
- The A record value matches `terraform output app_vm_ip`
- Cloudflare proxy is set to **DNS only** (grey cloud)

**`terraform output` errors**

Make sure `terraform apply` has been run and succeeded in
`btc-giftcard-infrastructure/envs/testing/`. Pass `--infra-dir` if the infra
repo is not in the default location.

**IAP tunnel refused**

Your Google account needs `roles/iap.tunnelResourceAccessor` on the project:
```bash
gcloud projects add-iam-policy-binding hispanie \
  --member="user:you@example.com" \
  --role="roles/iap.tunnelResourceAccessor"
```

---

## regtest-setup.sh

Bootstraps the full regtest environment in a single command. Designed for local
development; can also be run on the testing VM after `setup-vm.sh --init`.

### What it does

1. Tears down any existing stack and wipes all regtest Docker volumes (unless `--no-reset`)
2. Starts `bitcoind` in isolation and mines 1 block to exit IBD mode
3. Starts all remaining services (LND nodes, Postgres, Redis, API, workers)
4. Waits for the API and operator LND to be healthy
5. Mines 101 blocks to the operator wallet (funds the wallet; satisfies coinbase maturity)
6. Connects the operator and customer LND nodes as peers
7. Opens a 500,000 sat channel from operator to customer
8. Mines 6 blocks to activate the channel
9. Prints balances and a ready-to-use curl example

### Prerequisites

- Docker Engine + Docker Compose v2
- `curl`
- `jq`

No local Go installation needed — all services run inside containers.

### Usage

```bash
# Recommended: fresh start (wipes all regtest volumes)
./scripts/regtest-setup.sh

# Resume an existing running stack without wiping volumes
./scripts/regtest-setup.sh --no-reset
```

> **Why wipe by default?** `bitcoind`, `lnd`, and `lnd_customer` each store state
> in separate Docker named volumes. If any one is reset without the others, LND's
> stored wallet birthday will be higher than the current chain height, causing an
> unrecoverable sync loop:
> `Unable to synchronize wallet to chain: -8: Block height out of range`
>
> Wiping all volumes together on every fresh start guarantees they are always in
> sync from block 0.

### Start / stop / reset

```bash
# Start (fresh)
./scripts/regtest-setup.sh

# Start without wiping (resume)
./scripts/regtest-setup.sh --no-reset

# Stop (keep volumes)
docker compose -f docker-compose.yml -f docker-compose.regtest.yml stop

# Stop and wipe everything (manual equivalent of the default reset)
docker compose -f docker-compose.yml -f docker-compose.regtest.yml down -v --remove-orphans

# View logs
docker compose -f docker-compose.yml -f docker-compose.regtest.yml logs -f

# View logs for a specific service
docker compose -f docker-compose.yml -f docker-compose.regtest.yml logs -f api
```

### Service port map

| Service | Host port | Purpose |
|---------|-----------|---------|
| `api` | 3202 | btc-giftcard HTTP API |
| `postgres` | 5432 | Application database |
| `redis` | 6379 | Cache + Redis Streams |
| `lnd` | 10009 | Operator LND gRPC |
| `lnd` | 9735 | Operator LND P2P |
| `bitcoind` | 18443 | Regtest RPC |
| `bitcoind` | 18444 | Regtest P2P |

### Development workflow

```bash
# 1. Start environment
./scripts/regtest-setup.sh

# 2. Create a gift card (Stripe checkout is mocked in regtest)
curl -X POST http://localhost:3202/api/cards \
  -H 'Content-Type: application/json' \
  -d '{"fiat_amount_cents":1000,"fiat_currency":"EUR","purchase_email":"test@example.com"}' | jq .

# 3. Generate a Lightning invoice on the customer node
#    Replace <SATS> with btc_amount_sats from the card response
docker compose -f docker-compose.yml -f docker-compose.regtest.yml \
  exec lnd_customer lncli --network=regtest addinvoice --amt <SATS>

# 4. Redeem the card
curl -X POST http://localhost:3202/api/cards/<CODE>/redeem \
  -H 'Content-Type: application/json' \
  -d '{"method":"lightning","invoice":"<BOLT11>"}' | jq .

# 5. Mine blocks whenever you need on-chain confirmations
docker compose -f docker-compose.yml -f docker-compose.regtest.yml \
  exec bitcoind bitcoin-cli -regtest -rpcuser=btc -rpcpassword=btc \
  generatetoaddress 6 <ADDRESS>
```

### On-VM usage (testing environment)

After `setup-vm.sh --init`, SSH into the VM and run:

```bash
gcloud compute ssh btcgifter-app-vm --zone=europe-west4-a --project=hispanie \
  --tunnel-through-iap

# Inside the VM:
cd /opt/gifter/btc-giftcard
./scripts/regtest-setup.sh
```

The docker-compose stack started by `setup-vm.sh` is not pre-bootstrapped
(no blocks mined, no channel open). Running `regtest-setup.sh` after `--init`
completes the LND setup. This matches the process documented in
[`btc-giftcard-infrastructure/runbooks/lnd-wallet-init.md`](https://github.com/DanielDucuara2018/btc-giftcard-infrastructure/blob/main/runbooks/lnd-wallet-init.md).

### Troubleshooting

**`Block height out of range` loop in LND logs**

LND's wallet birthday is higher than bitcoind's chain height — the volumes are
out of sync. Fix: stop everything and run a full reset:
```bash
./scripts/regtest-setup.sh   # default wipes volumes
```

**API health check fails during setup**

The script waits up to 120 seconds for `GET /health` to return 200. If it times
out, check the API logs:
```bash
docker compose -f docker-compose.yml -f docker-compose.regtest.yml logs api
```

Common causes: missing `.env` variable, database migration failure, or port
conflict on 3202.

**`lnd-creds-exporter` fails**

LND generates TLS and macaroon files on first boot; `lnd-creds-exporter` copies
them to `./lnd-creds/`. If this service exits with an error, LND may not have
started correctly. Check:
```bash
docker compose -f docker-compose.yml -f docker-compose.regtest.yml logs lnd
docker compose -f docker-compose.yml -f docker-compose.regtest.yml logs lnd-creds-exporter
```
