#!/usr/bin/env bash
# Run the portable Go checks from .github/workflows/ci.yml locally.
set -euo pipefail

cd "$(dirname "$0")/.."

printf '%s\n' '==> gofmt -s'
unformatted=$(gofmt -s -l .)
if [[ -n "$unformatted" ]]; then
	printf 'needs gofmt -s:\n%s\n' "$unformatted"
	exit 1
fi

printf '%s\n' '==> go vet'
go vet ./...

printf '%s\n' '==> golangci-lint'
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1 run ./...

printf '%s\n' '==> go mod tidy'
go mod tidy
git diff --exit-code -- go.mod go.sum

printf '%s\n' '==> race tests and coverage'
pkgs=()
while IFS= read -r pkg; do
	pkgs+=("$pkg")
done < <(go list ./... | grep -v -e /internal/browser -e /internal/tui)
coverpkg=$(IFS=,; echo "${pkgs[*]}")
go test -race -shuffle=on -coverprofile=cover.out -coverpkg="$coverpkg" "${pkgs[@]}"
total=$(go tool cover -func=cover.out | awk '/^total:/ {gsub("%", "", $3); print $3}')
rm -f cover.out
printf 'total coverage: %s%%\n' "$total"
awk -v t="$total" -v floor=90 'BEGIN { if (t+0 < floor) { printf "coverage %.1f%% is below the %d%% floor\n", t, floor; exit 1 } }'

printf '%s\n' '==> cross-compile'
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
	GOOS=${target%/*} GOARCH=${target#*/} CGO_ENABLED=0 go build -trimpath -o /dev/null ./cmd/whip
done

printf '%s\n' '==> govulncheck'
go run golang.org/x/vuln/cmd/govulncheck@latest ./...

printf '%s\n' 'Local CI checks passed.'
