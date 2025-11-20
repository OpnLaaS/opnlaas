# OpnLaaS

## Deployment Setup

We recommend you use this in conjunction with a FreeIPA domain for authentication, permissions, and SSL/TLS management.

### HTTPS Setup via FreeIPA (Recommended)

1. Ensure your OpnLaaS server is joined to your FreeIPA domain.
2. Create the SSL directory `sudo mkdir -p /etc/ssl/ipa`.
3. Set the correct permissions: `sudo chown root:root /etc/ssl/ipa && sudo chmod 755 /etc/ssl/ipa`.
4. Request a certificate: `sudo ipa-getcert request -f /etc/ssl/ipa/opnlaas.crt -k /etc/ssl/ipa/opnlaas.key -N "CN=$(hostname -f)" -D $(hostname -f) -K "HTTP/$(hostname -f)" -w`.
5. Verify the certificate is issued: `sudo ipa-getcert list`.

## Installation/Update

We have a handy-dandy installation script located in the `scripting` directory. You can use it to install, update, or uninstall OpnLaaS.

Let's make this even easier! You can run the following command to download and execute the installer script in one go:

```bash
curl -sSL https://raw.githubusercontent.com/opnlaas/opnlaas/main/scripting/laas_installer.sh | bash -s -- -u
```

## Development Setup

Requirements:
- `Go` version 1.24.0 or higher
- `npm` version 10.0.0 or higher

Running:
1. Two-shell setup (recommended):
    - In the first shell, run `npm run devel` to set up the Tailwind CSS watcher.
    - In the second shell, run `go run main.go` to start the OpnLaaS server.
2. Single-shell setup:
    - Run `npm run devel &` to start the Tailwind CSS watcher in the background.
    - Then run `go run main.go` to start the OpnLaaS server.

Populating your development database:

Run: `go run tests/dev_setup/main.go`