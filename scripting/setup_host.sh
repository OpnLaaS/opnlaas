#!/usr/bin/env bash
set -euo pipefail

# This script prepares the host OS with the PXE prerequisites:
# * loop module with max_loop=64
# * syslinux binaries (pxelinux.0, ldlinux.c32) under /var/lib/tftpboot

TFTP_ROOT=${TFTP_ROOT:-/var/lib/tftpboot}
PXELINUX_CFG="${TFTP_ROOT}/pxelinux.cfg"
LOOP_CONF="/etc/modprobe.d/loop.conf"

log() { printf '[host-setup] %s\n' "$*"; }
warn() { printf '[host-setup] WARN: %s\n' "$*" >&2; }

require_root() {
	if [[ $EUID -ne 0 ]]; then
		warn "Must run as root to configure host prerequisites. Skipping."
		exit 0
	fi
}

ensure_loop_module() {
	if lsmod | grep -q '^loop'; then
		if ! grep -qs 'max_loop=64' "$LOOP_CONF" 2>/dev/null; then
			log "Ensuring loop module configuration at $LOOP_CONF"
			echo 'options loop max_loop=64' >"$LOOP_CONF"
			modprobe -r loop || true
			modprobe loop
		else
			log "Loop module already configured"
		fi
		return
	fi

	log "Loading loop module"
	if ! modprobe loop; then
		if command -v dnf >/dev/null 2>&1; then
			log "Installing kernel-modules via dnf"
			dnf install -y kernel-modules
			modprobe loop
		else
			warn "dnf not available; cannot load loop module automatically"
		fi
	fi

	if ! grep -qs 'max_loop=64' "$LOOP_CONF" 2>/dev/null; then
		log "Writing $LOOP_CONF"
		echo 'options loop max_loop=64' >"$LOOP_CONF"
	fi
}

ensure_syslinux() {
	if [[ ! -d $TFTP_ROOT ]]; then
		log "Creating $TFTP_ROOT"
		mkdir -p "$TFTP_ROOT"
	fi
	mkdir -p "$PXELINUX_CFG"

	if [[ -f ${TFTP_ROOT}/pxelinux.0 ]] && [[ -f ${TFTP_ROOT}/ldlinux.c32 ]]; then
		log "Syslinux artifacts already present"
		return
	fi

	if ! command -v dnf >/dev/null 2>&1; then
		warn "dnf not available; cannot install syslinux automatically"
		return
	fi

	log "Installing syslinux via dnf"
	dnf install -y syslinux

	if [[ -f /usr/share/syslinux/pxelinux.0 ]]; then
		cp /usr/share/syslinux/pxelinux.0 "$TFTP_ROOT/"
	fi
	if [[ -f /usr/share/syslinux/ldlinux.c32 ]]; then
		cp /usr/share/syslinux/ldlinux.c32 "$TFTP_ROOT/"
	fi
}

main() {
	if [[ ${CI:-} == "true" ]]; then
		log "Running in CI; skipping host modifications"
		exit 0
	fi

	require_root
	ensure_loop_module
	ensure_syslinux
	log "Host PXE prerequisites verified"
}

main "$@"
