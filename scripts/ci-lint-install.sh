source scripts/lib.sh

# TODO(henvic): Version these developer dependencies.

ensure_go_binary honnef.co/go/tools/cmd/staticcheck
ensure_go_binary github.com/securego/gosec/v2/cmd/gosec
ensure_go_binary golang.org/x/vuln/cmd/govulncheck
