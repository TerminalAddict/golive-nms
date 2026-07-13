#!/bin/sh
set -eu
test "$(id -u)" -eq 0 || { echo "Run this installer as root" >&2; exit 1; }
install -m 0755 golive-collector /usr/bin/golive-collector
install -d -m 0700 /var/lib/golive-collector
if [ ! -f /etc/golive-collector.env ]; then install -m 0600 golive-collector.env.example /etc/golive-collector.env; fi
if command -v systemctl >/dev/null 2>&1; then install -m 0644 golive-collector.service /etc/systemd/system/golive-collector.service; systemctl daemon-reload; systemctl enable golive-collector
elif command -v rc-update >/dev/null 2>&1; then install -m 0755 golive-collector.openrc /etc/init.d/golive-collector; rc-update add golive-collector default
else echo "Configure your init system to run /usr/bin/golive-collector"; fi
echo "Edit /etc/golive-collector.env, then start the collector."
