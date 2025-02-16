#!/bin/bash
set -euo pipefail
IFS=$'\n\t'

# Static analysis scripts
cd $(dirname $0)/..

source scripts/ci-lint-fmt.sh

set -x
go vet ./...
go tool honnef.co/go/tools/cmd/staticcheck ./...
go tool github.com/securego/gosec/v2/cmd/gosec -quiet -exclude-generated ./...
# Run govulncheck only informationally for the time being.
go tool golang.org/x/vuln/cmd/govulncheck ./... || true
