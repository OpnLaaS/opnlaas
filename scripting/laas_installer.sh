#!/bin/bash

# https://raw.githubusercontent.com/opnlaas/opnlaas/main/scripting/laas_installer.sh
# This script downloads and installs the OpnLaaS software to a supported Linux system.

GITHUB_REPOSITORY="opnlaas/opnlaas"
RELEASES_URL="https://api.github.com/repos/${GITHUB_REPOSITORY}/releases"

set -e
set -o pipefail

# Function to confirm a value, returns the value confirmed (aka, yes/no, if no ask for new value)
util_confirm_value() {
    local prompt_message="$1"
    local current_value="$2"
    local user_input

    while true; do
        read -rp "${prompt_message} [${current_value}]: " user_input
        user_input="${user_input:-$current_value}"

        read -rp "You entered '${user_input}'. Is this correct? (y/n): " confirmation
        case $confirmation in
            [Yy]* ) echo "$user_input"; return ;;
            [Nn]* ) echo "Let's try again." ;;
            * ) echo "Please answer Y/y or N/n." ;;
        esac
    done
}

# Function to ensure prerequisites are met
ensure_prereqs() {
    local prereqs=(jq wget nano tar)

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
    echo "  -u, --update        Update to the latest release. Can also be used to install to the latest version."
    echo "  -p, --purge         Uninstall OpnLaaS from the system"
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
    local download_url=$(wget -q -O - "${RELEASES_URL}/tags/${release_tag}" | jq -r '.assets[0].browser_download_url')

    local install_dir=$(util_confirm_value "Enter installation directory" "/opt/opnlaas")
    local laas_user="opnlaas"

    # Create installation directory if it doesn't exist
    sudo mkdir -p "$install_dir"
    sudo chown "$(whoami)":"$(whoami)" "$install_dir"

    # Download and extract the release
    wget -q -O - "$download_url" | tar -xz -C "$install_dir"

    # Create a dedicated user for OpnLaaS (if not exists)
    if ! id -u "$laas_user" &> /dev/null; then
        sudo useradd -r -s /bin/false "$laas_user"
    fi

    # Set ownership of installation directory
    sudo chown -R "$laas_user":"$laas_user" "$install_dir"

    # LaaS needs to write to /var/lib/tftpboot and /var/www/tftpboot,
    # so we need to give the opnlaas user ownership of these directories.
    sudo mkdir -p /var/lib/tftpboot
    sudo mkdir -p /var/www/tftpboot
    sudo chown -R "$laas_user":"$laas_user" /var/lib/tftpboot
    sudo chown -R "$laas_user":"$laas_user" /var/www/tftpboot

    # The opnlaas user should be allowed to bind on low ports if needed
    sudo setcap 'cap_net_bind_service=+ep' "${install_dir}/opnlaas"

    # Create a systemd service file
    local service_file="/etc/systemd/system/opnlaas.service"
    sudo bash -c "cat > $service_file" <<EOL
[Unit]
Description=OpnLaaS Service
After=network.target

[Service]
Type=simple
User=${laas_user}
ExecStart=${install_dir}/opnlaas
WorkingDirectory=${install_dir}
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOL

    # Reload systemd, enable and start the service
    sudo systemctl daemon-reload

    # The first run of the opnlaas binary makes the .env file if it doesn't exist. So we check if .env exists, if not we run the binary once to create it.
    if [ ! -f "${install_dir}/.env" ]; then
        echo "Creating initial .env file..."
        # This will error out, it shouldn't crash the script though. We just want to create the .env file. Make sure not to let it output to the user.
        sudo su - "$laas_user" -s /bin/bash -c "cd ${install_dir} && ./opnlaas" &> /dev/null || true
    fi

    # echo "Please configure the .env file now. After exiting the editor, the OpnLaaS service will start."
    # read -rp "Press Enter to continue..."
    # sudo nano "${install_dir}/.env"

    # Ask to configure the .env file (y/n)
    read -rp "Would you like to configure the .env file now? (y/n) (If this is an initial setup, this is highly recommended!): " configure_env
    if [[ "$configure_env" =~ ^[Yy]$ ]]; then
        sudo nano "${install_dir}/.env"
    fi

    sudo systemctl enable --now opnlaas.service

    echo "OpnLaaS version ${release_tag} installed successfully in ${install_dir}."

    exit 0
}

# Function to uninstall OpnLaaS
uninstall_opnlaas() {
    local install_dir=$(util_confirm_value "Enter installation directory to remove" "/opt/opnlaas")
    local laas_user="opnlaas"

    # Stop and disable the service
    sudo systemctl stop opnlaas.service || true
    sudo systemctl disable opnlaas.service || true

    # Remove systemd service file
    sudo rm -f /etc/systemd/system/opnlaas.service
    sudo systemctl daemon-reload

    # Remove installation directory
    sudo rm -rf "$install_dir"

    # Remove dedicated user
    sudo userdel "$laas_user" || true

    # Remove capabilities
    sudo setcap -r "${install_dir}/opnlaas" || true
    
    echo "OpnLaaS uninstalled successfully from ${install_dir}."
    
    exit 0
}

# Main script execution starts here
ensure_prereqs

# Parse command-line arguments
while [[ "$#" -gt 0 ]]; do
    case $1 in
        -h|--help) usage ;;
        -r|--releases) list_releases ;;
        -i|--install) 
            if [[ -n "$2" ]]; then
                install_release "$2"
                shift
            else
                echo "Error: --install requires a release tag argument."
                usage
            fi
            ;;
        -u|--update)
            latest_tag=$(wget -q -O - "${RELEASES_URL}" | jq -r '.[0].tag_name')
            echo "Updating to the latest release: ${latest_tag}"
            install_release "$latest_tag"
            ;;
        -p|--purge) uninstall_opnlaas ;;
        *) echo "Unknown parameter passed: $1"; usage ;;
    esac
    shift
done

usage