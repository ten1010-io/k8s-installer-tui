# Builds k8s-installer-tui for linux and runs simulate.sh against the installers
# checked out next to this repository (..\k8s-installer, ..\aipub-installer).
#
#   powershell -File test\simulate\run.ps1          # -Keep <dir> keeps the rendered files there
#
# Needs Docker Desktop and the image of aipub-installer\tools\chart-images
# (its run.ps1 builds it)
param([string]$Keep = "")
$ErrorActionPreference = "Stop"
$tui = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$gh = Split-Path $tui -Parent

docker run --rm -v "${tui}:/src" -v tui-gomod:/go/pkg/mod -w /src -e CGO_ENABLED=0 golang:1.22 `
    go build -o dist/k8s-installer-tui .
if ($LASTEXITCODE -ne 0) { throw "go build failed" }

$keepArgs = @()
if ($Keep) {
    New-Item -ItemType Directory -Force $Keep | Out-Null
    $keepArgs = @("-v", "$((Resolve-Path $Keep).Path):/keep", "-e", "KEEP=/keep")
}
docker run --rm -v "${gh}:/gh" @keepArgs aipub-chart-images bash /gh/k8s-installer-tui/test/simulate/simulate.sh
exit $LASTEXITCODE
