#!/bin/bash

# Global post install script. Install fastfetch/neofetch and run it at login.

## If using apt based system (Debian/Ubuntu)
if command -v apt-get >/dev/null 2>&1; then
    apt-get update
    apt-get install -y neofetch
    echo "neofetch" >> /etc/bash.bashrc
fi

## If using dnf based system (Fedora/CentOS)
if command -v dnf >/dev/null 2>&1; then
    dnf install -y fastfetch
    echo "fastfetch" >> /etc/bash.bashrc
fi
