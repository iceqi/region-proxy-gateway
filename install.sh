#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="/opt/region-proxy-gateway"
SERVICE_FILE="/etc/systemd/system/region-proxy-gateway.service"
REPO_URL="https://github.com/iceqi/region-proxy-gateway.git"

if [[ "${EUID}" -ne 0 ]]; then
  echo "error: run as root"
  exit 1
fi

install_packages() {
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update
    apt-get install -y openvpn iproute2 iptables ca-certificates curl git golang-go
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y openvpn iproute iptables ca-certificates curl git golang
  elif command -v yum >/dev/null 2>&1; then
    yum install -y epel-release || true
    yum install -y openvpn iproute iptables ca-certificates curl git golang
  elif command -v apk >/dev/null 2>&1; then
    apk add --no-cache openvpn iproute2 iptables ca-certificates curl git go bash
  else
    echo "unsupported package manager"
    exit 1
  fi
}

install_packages

mkdir -p "${INSTALL_DIR}"

if [[ -d "${INSTALL_DIR}/.git" ]]; then
  git -C "${INSTALL_DIR}" pull --ff-only
else
  git clone "${REPO_URL}" "${INSTALL_DIR}"
fi

cd "${INSTALL_DIR}"
go build -o region-proxy-gateway ./cmd/region-proxy-gateway

if command -v systemctl >/dev/null 2>&1; then
  cat >"${SERVICE_FILE}" <<EOF
[Unit]
Description=Region Proxy Gateway
After=network.target

[Service]
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/region-proxy-gateway
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

  systemctl daemon-reload
  systemctl enable region-proxy-gateway
  systemctl restart region-proxy-gateway
  echo "service started."
else
  echo "systemd not found. Run manually with: ${INSTALL_DIR}/region-proxy-gateway"
fi

echo "Admin: http://SERVER_IP:8787"
echo "HTTP proxy example: http://jp-10:PASSWORD@SERVER_IP:3000"
echo "SOCKS5 proxy example: socks5://jp-10:PASSWORD@SERVER_IP:3000"
