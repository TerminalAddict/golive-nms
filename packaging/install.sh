#!/bin/sh
set -eu
test "$(id -u)" -eq 0 || { echo "Run this installer as root" >&2; exit 1; }
install -m 0755 golive-agent /usr/bin/golive-agent
install -d -m 0700 /var/lib/golive-agent
if [ ! -f /etc/golive-agent.env ]; then install -m 0600 packaging/golive-agent.env.example /etc/golive-agent.env 2>/dev/null || install -m 0600 golive-agent.env.example /etc/golive-agent.env; fi
if command -v systemctl >/dev/null 2>&1; then
  install -m 0644 deploy/golive-agent.service /etc/systemd/system/golive-agent.service 2>/dev/null || install -m 0644 golive-agent.service /etc/systemd/system/golive-agent.service
  systemctl daemon-reload
  systemctl enable golive-agent
elif command -v rc-update >/dev/null 2>&1; then
  install -m 0755 deploy/golive-agent.openrc /etc/init.d/golive-agent 2>/dev/null || install -m 0755 golive-agent.openrc /etc/init.d/golive-agent
  rc-update add golive-agent default
else
  echo "Installed binary and configuration; configure your init system to run /usr/bin/golive-agent"
fi
echo "Edit /etc/golive-agent.env, then start golive-agent with your service manager."
