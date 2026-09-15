#!/usr/bin/env bash
set -euo pipefail

readonly service=my-agent
readonly binary=/usr/local/bin/my-agent
readonly unit=/etc/systemd/system/my-agent.service
readonly env_dir=/etc/my-agent
readonly env_file=/etc/my-agent/env
readonly home=/var/lib/my-agent

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
sudo=""
if [[ $EUID -ne 0 ]]; then
  sudo=sudo
fi

fail() {
  echo "deploy: $*" >&2
  exit 1
}

for command in go git systemctl; do
  command -v "$command" >/dev/null || fail "$command is not installed"
done
getent group docker >/dev/null || fail "the docker group is missing, install Docker Engine first"

if ! $sudo test -f "$env_file"; then
  $sudo install -d -m 0755 "$env_dir"
  $sudo install -m 0600 /dev/null "$env_file"
  $sudo tee "$env_file" >/dev/null <<'EOF'
OPENROUTER_API_KEY=
TELEGRAM_BOT_TOKEN=
TELEGRAM_ALLOWED_USERS=
GITHUB_TOKEN=
MY_AGENT_MODEL=
EOF
  fail "fill in $env_file, then run this script again"
fi

echo "==> building"
build=$(mktemp -d)
trap 'rm -rf "$build"' EXIT
(cd "$root" && go build -o "$build/$service" .)

if ! id -u "$service" >/dev/null 2>&1; then
  echo "==> creating the $service user"
  $sudo useradd --system --user-group --home-dir "$home" --no-create-home --shell /usr/sbin/nologin "$service"
fi

# The build comes first, so a build that fails leaves the running bot alone.
if systemctl is-active --quiet "$service"; then
  echo "==> stopping the running bot"
  $sudo systemctl stop "$service"
fi

echo "==> installing"
$sudo install -m 0755 "$build/$service" "$binary"
$sudo install -m 0644 "$root/deploy/$service.service" "$unit"
$sudo systemctl daemon-reload
$sudo systemctl enable --quiet "$service"

echo "==> starting"
$sudo systemctl start "$service"
sleep 3
if ! systemctl is-active --quiet "$service"; then
  $sudo journalctl -u "$service" -n 50 --no-pager >&2
  fail "$service did not stay up"
fi

$sudo systemctl status "$service" --no-pager -n 10
echo "==> deployed, follow the logs with: journalctl -u $service -f"
