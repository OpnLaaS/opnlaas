# OpnLaaS

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