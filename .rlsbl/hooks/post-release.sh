#!/usr/bin/env bash
# Post-release hook. Runs after a successful rlsbl release.

set -euo pipefail

echo "Installing wake v$RLSBL_VERSION..."
go install ./cmd/wake
echo "Installed: $(which wake)"
