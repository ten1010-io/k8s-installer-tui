# Builds k8s-installer-tui for linux and runs in-container.sh in throwaway
# containers with real sshd: --check-ssh and --send-ssh-key against users of the
# container's own sshd. Nothing outside the containers is touched.
#
#   powershell -File test\sshkeys\run.ps1 [-K8sInstaller <checkout>] [-Images ubuntu:24.04,rockylinux:8]
#
# Needs Docker Desktop and the network (packages, ansible-core from PyPI).
# configure-control-node-ssh.sh is taken from the k8s-installer checkout next to
# this repository unless given; it is mounted read only.
param(
    [string]$K8sInstaller = "",
    [string[]]$Images = @("ubuntu:24.04", "rockylinux:8")
)
$ErrorActionPreference = "Stop"
$tui = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
if (-not $K8sInstaller) { $K8sInstaller = Join-Path (Split-Path $tui -Parent) "k8s-installer" }
$configure = (Resolve-Path (Join-Path $K8sInstaller "ansible\configure-control-node-ssh.sh")).Path

docker run --rm -v "${tui}:/src" -v tui-gomod:/go/pkg/mod -w /src -e CGO_ENABLED=0 golang:1.22 `
    sh -c "go mod tidy && go build -o dist/k8s-installer-tui ."
if ($LASTEXITCODE -ne 0) { throw "go build failed" }

$failed = @()
foreach ($image in $Images) {
    Write-Host "════ $image"
    docker run --rm `
        -v "${tui}\dist\k8s-installer-tui:/src/k8s-installer-tui:ro" `
        -v "${configure}:/src/configure-control-node-ssh.sh:ro" `
        -v "${PSScriptRoot}\in-container.sh:/src/in-container.sh:ro" `
        $image bash /src/in-container.sh
    if ($LASTEXITCODE -ne 0) { $failed += $image }
}
if ($failed.Count -gt 0) { Write-Host "✗ failed: $($failed -join ', ')"; exit 1 }
Write-Host "✓ passed: $($Images -join ', ')"
