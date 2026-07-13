#!/bin/bash
# setup-vm.sh — Deploys the btc-giftcard regtest stack to the testing VM.
#
# VM connection details are read from Terraform outputs when an infra directory
# is found. Pass explicit flags to override, or when running outside Terraform.
#
# This script is relocatable: it works whether it lives in the infrastructure
# repo (scripts/) or in the app repo (scripts/). Terraform auto-detection checks
# both locations; pass --infra-dir to override.
#
# MODES
#   --init    First-time setup (nginx, certbot, start docker-compose)
#   --deploy  Update code and restart services
#   --status  Show docker-compose ps + API health check
#
# SOURCE — how code reaches the VM (choose one per invocation)
#   --repo  URL   Clone/pull from git (default; prompted on --init if omitted)
#   --local PATH  Sync from a local directory via tar+ssh
#                 WARNING: deploys uncommitted changes — development only
#
# VM CONNECTION — auto-detected from Terraform; override when needed
#   --infra-dir PATH   Path to btc-giftcard-infrastructure/ repo root
#   --vm-name   NAME   GCE instance name
#   --vm-zone   ZONE   GCE zone (e.g. europe-west4-a)
#   --project   ID     GCP project ID
#   --subdomain HOST   API hostname (e.g. api.gifter.danobhub.com)
#   --vm-ip     IP     VM external IP (used only for the DNS check prompt)
#
# OTHER
#   --email EMAIL   Let's Encrypt email (required for --init)
#   --env   PATH    Local .env file to upload to the VM
#
# Examples:
#   ./scripts/setup-vm.sh --init --email admin@danobhub.com
#   ./scripts/setup-vm.sh --init --local ~/dev/btc-giftcard --email admin@example.com
#   ./scripts/setup-vm.sh --deploy
#   ./scripts/setup-vm.sh --deploy --local ~/dev/btc-giftcard
#   ./scripts/setup-vm.sh --deploy --vm-name my-vm --vm-zone us-central1-a --project my-proj --subdomain api.example.com
#
# Prerequisites (first-time):
#   1. `terraform apply` has been run in envs/testing/ (or pass VM flags manually)
#   2. Cloudflare A record for the API subdomain points to the VM IP (DNS only, not proxied)
#   3. gcloud CLI is authenticated: gcloud auth login
#   4. IAP access: your account needs roles/iap.tunnelResourceAccessor on the project

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ── Defaults ──────────────────────────────────────────────────────────────────
MODE=""
SOURCE_MODE="git"        # git | local
LOCAL_APP_DIR=""
APP_REPO_URL="${APP_REPO_URL:-}"
CERTBOT_EMAIL=""
APP_ENV_FILE="${APP_ENV_FILE:-}"
APP_DIR_ON_VM="/opt/gifter/btc-giftcard"

# VM connection — populated from Terraform or CLI flags
INFRA_DIR_OVERRIDE=""
VM_NAME=""
VM_ZONE=""
PROJECT=""
API_SUBDOMAIN=""
VM_IP=""

# ── Argument parsing ──────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --init)       MODE="init";   shift ;;
    --deploy)     MODE="deploy"; shift ;;
    --status)     MODE="status"; shift ;;
    --local)      SOURCE_MODE="local"; LOCAL_APP_DIR="$2"; shift 2 ;;
    --repo)       SOURCE_MODE="git";   APP_REPO_URL="$2"; shift 2 ;;
    --email)      CERTBOT_EMAIL="$2"; shift 2 ;;
    --env)        APP_ENV_FILE="$2";  shift 2 ;;
    --infra-dir)  INFRA_DIR_OVERRIDE="$2"; shift 2 ;;
    --vm-name)    VM_NAME="$2";       shift 2 ;;
    --vm-zone)    VM_ZONE="$2";       shift 2 ;;
    --project)    PROJECT="$2";       shift 2 ;;
    --subdomain)  API_SUBDOMAIN="$2"; shift 2 ;;
    --vm-ip)      VM_IP="$2";         shift 2 ;;
    *)            echo "Unknown flag: $1"; exit 1 ;;
  esac
done

if [[ -z "${MODE}" ]]; then
  echo "Usage: $0 --init|--deploy|--status [OPTIONS]"
  echo ""
  echo "  Source:  --repo URL | --local PATH"
  echo "  VM:      --infra-dir PATH | --vm-name NAME --vm-zone ZONE --project ID --subdomain HOST"
  echo "  Other:   --email EMAIL --env PATH"
  exit 1
fi

if [[ "${SOURCE_MODE}" == "local" ]]; then
  if [[ -z "${LOCAL_APP_DIR}" ]]; then
    echo "ERROR: --local requires a path (e.g. --local ~/dev/btc-giftcard)"
    exit 1
  fi
  if [[ ! -d "${LOCAL_APP_DIR}" ]]; then
    echo "ERROR: --local path not found: ${LOCAL_APP_DIR}"
    exit 1
  fi
  LOCAL_APP_DIR="$(cd "${LOCAL_APP_DIR}" && pwd)"  # canonicalize
fi

# ── Resolve VM connection details ─────────────────────────────────────────────
resolve_vm_details() {
  # If all required values are already provided via CLI flags, skip Terraform.
  if [[ -n "${VM_NAME}" && -n "${VM_ZONE}" && -n "${PROJECT}" && -n "${API_SUBDOMAIN}" ]]; then
    echo "==> Using VM details from CLI flags."
    return 0
  fi

  # Auto-detect the infra directory. Checks two locations so the script works
  # whether it lives in the infrastructure repo (scripts/) or the app repo.
  local infra_dir="${INFRA_DIR_OVERRIDE:-}"
  if [[ -z "${infra_dir}" ]]; then
    for candidate in \
      "$(cd "${SCRIPT_DIR}/.." && pwd)" \
      "$(cd "${SCRIPT_DIR}/../.." && pwd)/btc-giftcard-infrastructure"; do
      if [[ -d "${candidate}/envs/testing" ]]; then
        infra_dir="${candidate}"
        break
      fi
    done
  fi

  if [[ -z "${infra_dir}" ]]; then
    cat >&2 <<EOF
ERROR: Cannot find btc-giftcard-infrastructure/ automatically.
       Pass --infra-dir PATH  to point to the infra repo, or supply all four
       VM flags explicitly: --vm-name NAME --vm-zone ZONE --project ID --subdomain HOST
EOF
    exit 1
  fi

  local tf_dir="${infra_dir}/envs/testing"
  echo "==> Reading Terraform outputs from ${tf_dir} ..."
  [[ -z "${VM_NAME}" ]]       && VM_NAME=$(terraform -chdir="${tf_dir}" output -raw app_vm_name)
  [[ -z "${VM_ZONE}" ]]       && VM_ZONE=$(terraform -chdir="${tf_dir}" output -raw app_vm_zone)
  [[ -z "${PROJECT}" ]]       && PROJECT=$(terraform -chdir="${tf_dir}" output -raw project_id)
  [[ -z "${API_SUBDOMAIN}" ]] && API_SUBDOMAIN=$(terraform -chdir="${tf_dir}" output -raw api_subdomain)
  [[ -z "${VM_IP}" ]]         && VM_IP=$(terraform -chdir="${tf_dir}" output -raw app_vm_ip)
}

resolve_vm_details

echo "    VM name:       ${VM_NAME}"
echo "    VM zone:       ${VM_ZONE}"
echo "    GCP project:   ${PROJECT}"
echo "    API subdomain: ${API_SUBDOMAIN}"
[[ -n "${VM_IP}" ]] && echo "    VM IP:         ${VM_IP}"

if [[ "${SOURCE_MODE}" == "local" ]]; then
  echo ""
  echo "  *** LOCAL SYNC MODE ***"
  echo "  Source:  ${LOCAL_APP_DIR}"
  echo "  WARNING: Uncommitted changes will be deployed. Not reproducible."
  echo "           Do NOT use this mode in production."
  echo ""
fi

# ── SSH / transfer helpers ────────────────────────────────────────────────────
vm_ssh() {
  gcloud compute ssh "${VM_NAME}" \
    --zone="${VM_ZONE}" \
    --project="${PROJECT}" \
    --tunnel-through-iap \
    -- "$@"
}

vm_scp() {
  gcloud compute scp \
    --zone="${VM_ZONE}" \
    --project="${PROJECT}" \
    --tunnel-through-iap \
    "$@"
}

# Sync a local directory to the VM by piping a tar archive over the IAP SSH
# connection. Uses gcloud compute ssh directly so OS Login username resolution
# is handled automatically (no manual key or username configuration needed).
#
# Excluded: .git, .env (uploaded separately via --env), lnd-creds (managed
# separately), tmp/, *.log, *.swp
#
# Note: files present on the VM but removed locally are NOT deleted (no --delete
# equivalent). For a clean slate, SSH in and rm -rf the app dir before syncing.
vm_sync_local() {
  local src="${LOCAL_APP_DIR}"
  local dst="${APP_DIR_ON_VM}"
  echo "==> Syncing local files to VM via tar+ssh ..."
  echo "    Source: ${src}/"
  echo "    Target: ${dst}"
  vm_ssh "sudo mkdir -p '${dst}' && sudo chown gifter:gifter '${dst}'"
  tar -czf - \
    --exclude='.git' \
    --exclude='.env' \
    --exclude='lnd-creds' \
    --exclude='tmp' \
    --exclude='*.log' \
    --exclude='*.swp' \
    -C "${src}" . | \
  gcloud compute ssh "${VM_NAME}" \
    --zone="${VM_ZONE}" \
    --project="${PROJECT}" \
    --tunnel-through-iap \
    -- "sudo tar -xzf - -C '${dst}' && sudo chown -R gifter:gifter '${dst}'"
  echo "    Sync complete."
}

# ── Wait for VM startup script to complete ────────────────────────────────────
wait_for_vm() {
  echo "==> Waiting for VM startup script to finish ..."
  local retries=30
  while [[ ${retries} -gt 0 ]]; do
    if vm_ssh "test -f /var/btcgifter-startup-done" 2>/dev/null; then
      echo "    VM is ready."
      return 0
    fi
    echo "    Not ready yet, retrying in 10s ... (${retries} attempts left)"
    sleep 10
    (( retries-- ))
  done
  echo "ERROR: VM did not become ready in time. Check startup logs:"
  echo "  gcloud compute instances get-serial-port-output ${VM_NAME} --zone=${VM_ZONE} --project=${PROJECT}"
  exit 1
}

# ── Deploy code — shared by --init and --deploy ───────────────────────────────
deploy_source() {
  if [[ "${SOURCE_MODE}" == "local" ]]; then
    vm_sync_local
  else
    if [[ -z "${APP_REPO_URL}" ]]; then
      read -rp "==> GitHub repo URL for btc-giftcard (e.g. https://github.com/you/btc-giftcard): " APP_REPO_URL
    fi
    echo "==> Deploying from git: ${APP_REPO_URL} ..."
    vm_ssh "
      if [ ! -d '${APP_DIR_ON_VM}/.git' ]; then
        sudo git clone '${APP_REPO_URL}' '${APP_DIR_ON_VM}'
        sudo chown -R gifter:gifter '${APP_DIR_ON_VM}'
      else
        cd '${APP_DIR_ON_VM}' && sudo -u gifter git pull
      fi
    "
  fi
}

# ── --init: First-time setup ──────────────────────────────────────────────────
if [[ "${MODE}" == "init" ]]; then

  if [[ -z "${CERTBOT_EMAIL}" ]]; then
    echo "ERROR: --email is required for --init (used for Let's Encrypt certificate)"
    exit 1
  fi

  wait_for_vm

  # Verify DNS is pointing to the VM before requesting a certificate
  echo ""
  echo "==> DNS check"
  echo "    Make sure Cloudflare A record for ${API_SUBDOMAIN} points to ${VM_IP:-<VM IP>}"
  echo "    before continuing, otherwise certbot will fail."
  echo ""
  read -rp "    DNS is pointing to the VM? (y/N): " dns_ok
  if [[ "${dns_ok}" != "y" && "${dns_ok}" != "Y" ]]; then
    echo "Aborted. Update DNS first, then re-run with --init."
    exit 0
  fi

  deploy_source

  # Upload .env
  if [[ -z "${APP_ENV_FILE}" ]]; then
    read -rp "==> Path to local .env file for btc-giftcard: " APP_ENV_FILE
  fi
  echo "==> Uploading .env ..."
  vm_scp "${APP_ENV_FILE}" "${VM_NAME}:/tmp/btcgifter.env"
  vm_ssh "sudo mv /tmp/btcgifter.env '${APP_DIR_ON_VM}/.env' && sudo chown gifter:gifter '${APP_DIR_ON_VM}/.env'"

  # Configure nginx
  echo "==> Configuring nginx (HTTP-only, for certbot challenge) ..."
  vm_ssh "sudo tee /etc/nginx/sites-available/btcgifter > /dev/null << 'NGINX'
server {
    listen 80;
    server_name ${API_SUBDOMAIN};

    location /.well-known/acme-challenge/ {
        root /var/www/certbot;
    }

    location / {
        proxy_pass http://127.0.0.1:3202;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_read_timeout 120s;
    }
}
NGINX
sudo ln -sf /etc/nginx/sites-available/btcgifter /etc/nginx/sites-enabled/btcgifter
sudo nginx -t && sudo systemctl reload nginx"

  # Obtain SSL certificate
  echo "==> Requesting Let's Encrypt certificate for ${API_SUBDOMAIN} ..."
  vm_ssh "sudo certbot --nginx -d '${API_SUBDOMAIN}' --non-interactive --agree-tos -m '${CERTBOT_EMAIL}'"
  echo "    Certificate installed. Nginx now serves HTTPS."

  # Start the regtest stack
  echo "==> Starting docker-compose regtest stack ..."
  vm_ssh "cd '${APP_DIR_ON_VM}' && sudo -u gifter docker compose -f docker-compose.yml -f docker-compose.regtest.yml up -d --build"

  echo ""
  echo "==> Init complete!"
  echo ""
  echo "Next steps:"
  echo "  1. Run the regtest setup script to mine initial blocks and fund the LND wallet:"
  echo "     SSH into the VM and run: cd ${APP_DIR_ON_VM} && ./scripts/regtest-setup.sh"
  echo "     Or SSH: gcloud compute ssh ${VM_NAME} --zone=${VM_ZONE} --project=${PROJECT} --tunnel-through-iap"
  echo ""
  echo "  2. Register the Stripe webhook in the Stripe dashboard:"
  echo "     Endpoint URL: https://${API_SUBDOMAIN}/api/webhook/stripe"
  echo "     Events:       checkout.session.completed, checkout.session.expired"
  echo ""
  echo "  3. Verify the API is reachable:"
  echo "     curl https://${API_SUBDOMAIN}/health"
fi

# ── --deploy: Update code and restart services ────────────────────────────────
if [[ "${MODE}" == "deploy" ]]; then

  wait_for_vm

  # Update .env if provided
  if [[ -n "${APP_ENV_FILE}" ]]; then
    echo "==> Uploading updated .env ..."
    vm_scp "${APP_ENV_FILE}" "${VM_NAME}:/tmp/btcgifter.env"
    vm_ssh "sudo mv /tmp/btcgifter.env '${APP_DIR_ON_VM}/.env' && sudo chown gifter:gifter '${APP_DIR_ON_VM}/.env'"
  fi

  deploy_source

  # Restart services (down + up to pick up env changes)
  echo "==> Restarting docker-compose stack ..."
  vm_ssh "
    cd '${APP_DIR_ON_VM}'
    sudo -u gifter docker compose -f docker-compose.yml -f docker-compose.regtest.yml down
    sudo -u gifter docker compose -f docker-compose.yml -f docker-compose.regtest.yml up -d --build
  "

  echo "==> Deploy complete."
fi

# ── --status: Check service health ───────────────────────────────────────────
if [[ "${MODE}" == "status" ]]; then

  echo "==> Docker service status:"
  vm_ssh "cd '${APP_DIR_ON_VM}' && sudo -u gifter docker compose -f docker-compose.yml -f docker-compose.regtest.yml ps"

  echo ""
  echo "==> API health check:"
  curl -sf "https://${API_SUBDOMAIN}/health" | jq . || echo "    API not reachable or not healthy"

fi
