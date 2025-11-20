#!/bin/bash

# This script downloads and installs the OpnLaaS software to a supported Linux system.

GITHUB_REPOSITORY="opnlaas/opnlaas"
RELEASES_URL="https://api.github.com/repos/${GITHUB_REPOSITORY}/releases"

set -e
set -o pipefail

# Function to ensure prerequisites are met (jq, wget)
ensure_prereqs() {
    local prereqs=(jq wget)

    for cmd in "${prereqs[@]}"; do
        if ! command -v "$cmd" &> /dev/null; then
            echo "Error: $cmd is not installed. Please install it and try again."
            exit 1
        fi
    done
}

# Function to display usage information
usage() {
    echo "Usage: $0 [options]"
    echo "Options:"
    echo "  -h, --help          Show this help message and exit"
    echo "  -r, --releases      List available releases"
    echo "  -i, --install <tag>  Install specified release tag"
    exit 0
}

# Function to display available releases (Format: #, Tag, Date)
list_releases() {
    echo "Available releases for ${GITHUB_REPOSITORY}:"
    
    wget -q -O - "${RELEASES_URL}" | jq -r '.[] | "\(.tag_name) \(.published_at)"' | nl -w2 -s'. '

    exit 0
}

# Function to install a specified release
install_release() {
    local release_tag="$1"
    local download_url

    download_url=$(wget -q -O - "${RELEASES_URL}/tags/${release_tag}" | jq -r '.assets[0].browser_download_url')

    if [[ -z "$download_url" || "$download_url" == "null" ]]; then
        echo "Error: Release tag '${release_tag}' not found."
        exit 1
    fi

    echo "Downloading release '${release_tag}' from ${download_url}..."
    wget -O "opnlaas_${release_tag}.tar.gz" "$download_url"

    echo "Extracting package..."
    tar -xzf "opnlaas_${release_tag}.tar.gz"

    echo "Installing OpnLaaS..."
    cd "opnlaas_${release_tag}" || exit 1
    sudo ./install.sh

    echo "OpnLaaS version '${release_tag}' installed successfully."
    exit 0
}

# Main script execution starts here
ensure_prereqs

# Parse command-line arguments
while [[ "$#" -gt 0 ]]; do
    case $1 in
        -h|--help) usage ;;
        -r|--releases) list_releases ;;
        *) echo "Unknown parameter passed: $1"; usage ;;
    esac
    shift
done

usage