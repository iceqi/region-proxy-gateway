#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="${INSTALL_DIR:-/opt/region-proxy-gateway}"
SERVICE_NAME="${SERVICE_NAME:-region-proxy-gateway}"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
REPO_URL="${REPO_URL:-https://github.com/iceqi/region-proxy-gateway.git}"
BRANCH="${BRANCH:-main}"
CONFIG_FILE="${INSTALL_DIR}/data/config.json"

if [[ "${EUID}" -ne 0 ]]; then
	if [[ "${RPG_INSTALL_TEST:-0}" != "1" ]]; then
		echo "error: please run as root"
		exit 1
	fi
fi

log() {
  echo "[region-proxy-gateway] $*"
}

install_packages() {
  log "installing dependencies"
  if command -v apt-get >/dev/null 2>&1; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get update
    apt-get install -y openvpn iproute2 iptables ca-certificates curl git golang-go jq gcc libc6-dev libsqlite3-dev
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y openvpn iproute iptables ca-certificates curl git golang jq gcc sqlite-devel
  elif command -v yum >/dev/null 2>&1; then
    yum install -y epel-release || true
    yum install -y openvpn iproute iptables ca-certificates curl git golang jq gcc sqlite-devel
  elif command -v apk >/dev/null 2>&1; then
    apk add --no-cache openvpn iproute2 iptables ca-certificates curl git go jq bash gcc musl-dev sqlite-dev
  else
    echo "unsupported package manager"
    exit 1
  fi
}

random_string() {
	local length="${1:-24}"
	local value
	set +o pipefail
	value="$(LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c "${length}")"
	set -o pipefail
	echo "${value}"
}

port_free() {
  local port="$1"
  if command -v ss >/dev/null 2>&1; then
    ! ss -ltn "sport = :${port}" | tail -n +2 | grep -q .
    return
  fi
  if command -v netstat >/dev/null 2>&1; then
    ! netstat -ltn | awk '{print $4}' | grep -Eq "[:.]${port}$"
    return
  fi
  return 0
}

random_free_port() {
	local port
	for _ in $(seq 1 200); do
		port="$((20000 + $(od -An -N2 -tu2 /dev/urandom | tr -d ' ') % 41000))"
		if port_free "${port}"; then
			echo "${port}"
			return 0
    fi
  done
  echo "8787"
}

server_ip() {
  local ip=""
  ip="$(curl -fsS --max-time 5 https://api.ipify.org 2>/dev/null || true)"
  if [[ -z "${ip}" ]]; then
    ip="$(hostname -I 2>/dev/null | awk '{print $1}' || true)"
  fi
  echo "${ip:-SERVER_IP}"
}

clone_or_update() {
  log "downloading source"
  if [[ -d "${INSTALL_DIR}/.git" ]]; then
    git -C "${INSTALL_DIR}" fetch origin "${BRANCH}"
    git -C "${INSTALL_DIR}" checkout "${BRANCH}"
    git -C "${INSTALL_DIR}" pull --ff-only origin "${BRANCH}"
  else
    mkdir -p "$(dirname "${INSTALL_DIR}")"
    git clone --branch "${BRANCH}" "${REPO_URL}" "${INSTALL_DIR}"
  fi
}

build_binary() {
  log "building binary"
  cd "${INSTALL_DIR}"
  go build -o region-proxy-gateway ./cmd/region-proxy-gateway
}

write_or_patch_config() {
  mkdir -p "${INSTALL_DIR}/data"

  local admin_port admin_path admin_user admin_pass proxy_user proxy_pass
  admin_port="$(random_free_port)"
  admin_path="/admin-$(random_string 20)"
  admin_user="admin"
  admin_pass="$(random_string 24)"
  proxy_user="proxy"
  proxy_pass="$(random_string 24)"

  if [[ ! -f "${CONFIG_FILE}" ]]; then
    log "creating config"
    cat >"${CONFIG_FILE}" <<EOF
{
  "admin_host": "0.0.0.0",
  "admin_port": ${admin_port},
  "admin_path": "${admin_path}",
  "admin_username": "${admin_user}",
  "admin_password": "${admin_pass}",
  "proxy_username": "${proxy_user}",
  "proxy_password": "${proxy_pass}",
  "node_refresh_interval": "20m",
  "data_dir": "${INSTALL_DIR}/data",
  "database_path": "${INSTALL_DIR}/data/region-proxy-gateway.db",
  "openvpn_command": "openvpn",
  "tunnel_backend": "openvpn",
  "channels": [
    {
      "id": "jp-3000",
      "listen_host": "0.0.0.0",
      "listen_port": 3000,
      "region": "jp",
      "rotate_minutes": 10,
      "selection_mode": "auto",
      "enabled": true
    }
  ]
}
EOF
    chmod 600 "${CONFIG_FILE}"
    return
  fi

	log "keeping existing config and patching install defaults"
	local tmp new_admin_path new_admin_pass new_proxy_pass
	tmp="$(mktemp)"
	new_admin_path="/admin-$(random_string 20)"
	new_admin_pass="$(random_string 24)"
	new_proxy_pass="$(random_string 24)"
	jq \
		--arg data_dir "${INSTALL_DIR}/data" \
		--arg database_path "${INSTALL_DIR}/data/region-proxy-gateway.db" \
		--arg admin_path "${new_admin_path}" \
		--arg admin_pass "${new_admin_pass}" \
		--arg proxy_pass "${new_proxy_pass}" '
    .admin_host = (.admin_host // "0.0.0.0") |
    .admin_host = (if .admin_host == "127.0.0.1" then "0.0.0.0" else .admin_host end) |
    .admin_path = (if (.admin_path == null or .admin_path == "" or .admin_path == "null") then $admin_path else .admin_path end) |
    .admin_username = (if (.admin_username == null or .admin_username == "") then "admin" else .admin_username end) |
    .admin_password = (if (.admin_password == null or .admin_password == "" or .admin_password == "change-me-admin") then $admin_pass else .admin_password end) |
    .proxy_username = (if (.proxy_username == null or .proxy_username == "") then "proxy" else .proxy_username end) |
    .proxy_password = (if (.proxy_password == null or .proxy_password == "" or .proxy_password == "change-me-proxy") then $proxy_pass else .proxy_password end) |
    .data_dir = $data_dir |
    .database_path = (.database_path // $database_path) |
    .openvpn_command = (.openvpn_command // "openvpn") |
    .tunnel_backend = "openvpn" |
    .node_refresh_interval = (.node_refresh_interval // "20m")
  ' "${CONFIG_FILE}" >"${tmp}"
  cat "${tmp}" >"${CONFIG_FILE}"
  rm -f "${tmp}"
  chmod 600 "${CONFIG_FILE}"
}

install_service() {
  if ! command -v systemctl >/dev/null 2>&1; then
    log "systemd not found. Run manually with: ${INSTALL_DIR}/region-proxy-gateway"
    return
  fi

  log "installing systemd service"
  cat >"${SERVICE_FILE}" <<EOF
[Unit]
Description=Region Proxy Gateway
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/region-proxy-gateway
Restart=always
RestartSec=5
LimitNOFILE=1048576
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW

[Install]
WantedBy=multi-user.target
EOF

  systemctl daemon-reload
  systemctl enable "${SERVICE_NAME}"
  systemctl restart "${SERVICE_NAME}"
}

print_summary() {
  local ip admin_host admin_port admin_path admin_user admin_pass proxy_user proxy_pass
  ip="$(server_ip)"
  admin_host="$(jq -r '.admin_host // "0.0.0.0"' "${CONFIG_FILE}")"
  admin_port="$(jq -r '.admin_port' "${CONFIG_FILE}")"
  admin_path="$(jq -r '.admin_path' "${CONFIG_FILE}")"
  admin_user="$(jq -r '.admin_username' "${CONFIG_FILE}")"
  admin_pass="$(jq -r '.admin_password' "${CONFIG_FILE}")"
  proxy_user="$(jq -r '.proxy_username' "${CONFIG_FILE}")"
  proxy_pass="$(jq -r '.proxy_password' "${CONFIG_FILE}")"

  echo
  echo "Install finished."
  echo "Config: ${CONFIG_FILE}"
  echo "Service: ${SERVICE_NAME}"
  if [[ "${admin_host}" == "127.0.0.1" ]]; then
    echo "Admin: http://127.0.0.1:${admin_port}${admin_path}"
  else
    echo "Admin: http://${ip}:${admin_port}${admin_path}"
  fi
  echo "Admin user: ${admin_user}"
  echo "Admin password: ${admin_pass}"
  echo "HTTP proxy example: http://${proxy_user}:${proxy_pass}@${ip}:3000"
  echo "SOCKS5 proxy example: socks5://${proxy_user}:${proxy_pass}@${ip}:3000"
  echo
  echo "Useful commands:"
  echo "  systemctl status ${SERVICE_NAME}"
  echo "  journalctl -u ${SERVICE_NAME} -f"
  echo "  systemctl restart ${SERVICE_NAME}"
}

if [[ "${RPG_INSTALL_TEST:-0}" != "1" ]]; then
	install_packages
	clone_or_update
	build_binary
	write_or_patch_config
	install_service
	print_summary
fi
