dev:
    #!/usr/bin/env bash
    set -euo pipefail
    tmp=$(mktemp)
    trap 'rm -f "$tmp"' EXIT
    GOOS=linux GOARCH=arm64 go build -o "$tmp" ./cmd/parro
    rsync "$tmp" root@claw:/usr/local/bin/parro
