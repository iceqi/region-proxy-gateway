#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORK_DIR="$(mktemp -d)"
trap 'rm -rf "${WORK_DIR}"' EXIT

export RPG_INSTALL_TEST=1
export INSTALL_DIR="${WORK_DIR}/app"
mkdir -p "${INSTALL_DIR}/data"

cat >"${INSTALL_DIR}/data/config.json" <<'JSON'
{
  "admin_host": "127.0.0.1",
  "admin_port": 37499,
  "admin_path": null,
  "admin_username": "admin",
  "admin_password": "change-me-admin",
  "proxy_username": "proxy",
  "proxy_password": "change-me-proxy",
  "channels": [
    {
      "id": "jp-3000",
      "listen_host": "0.0.0.0",
      "listen_port": 3000,
      "region": "jp",
      "selection_mode": "auto",
      "enabled": true
    }
  ]
}
JSON

# shellcheck source=/dev/null
source "${ROOT_DIR}/install.sh"

write_or_patch_config

admin_host="$(jq -r '.admin_host' "${INSTALL_DIR}/data/config.json")"
admin_path="$(jq -r '.admin_path' "${INSTALL_DIR}/data/config.json")"
admin_pass="$(jq -r '.admin_password' "${INSTALL_DIR}/data/config.json")"
proxy_pass="$(jq -r '.proxy_password' "${INSTALL_DIR}/data/config.json")"
backend="$(jq -r '.tunnel_backend' "${INSTALL_DIR}/data/config.json")"

if [[ "${admin_host}" != "0.0.0.0" ]]; then
  echo "admin_host = ${admin_host}, want 0.0.0.0"
  exit 1
fi
if [[ ! "${admin_path}" =~ ^/admin-[A-Za-z0-9]{20}$ ]]; then
  echo "admin_path = ${admin_path}, want random /admin-*"
  exit 1
fi
if [[ "${admin_pass}" == "change-me-admin" || -z "${admin_pass}" ]]; then
  echo "admin password was not rotated"
  exit 1
fi
if [[ "${proxy_pass}" == "change-me-proxy" || -z "${proxy_pass}" ]]; then
  echo "proxy password was not rotated"
  exit 1
fi
if [[ "${backend}" != "openvpn" ]]; then
  echo "backend = ${backend}, want openvpn"
  exit 1
fi
