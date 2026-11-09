#!/usr/bin/env bash
# Feeds each fixture to k8s-installer-tui --render and checks what comes out with
# the installers: check_installers.py (k8s-installer validator, aipub site value
# rules), then tools/chart-images/check.sh of aipub-installer with the rendered
# files/aipub/vars.yml as the site values, which renders every chart with them.
#
# Runs in the image of aipub-installer/tools/chart-images (python, ansible, helm),
# with the repositories side by side under /gh and the tool built for linux at
# /gh/k8s-installer-tui/dist/k8s-installer-tui (run.ps1 does all of it)
set -uo pipefail
GH=/gh
TUI="${GH}/k8s-installer-tui/dist/k8s-installer-tui"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORK=$(mktemp -d); trap 'rm -rf "${WORK}"' EXIT
pip install -q pydantic==2.12.0 2>/dev/null
fail=0

for fixture in "${HERE}"/fixtures/*/; do
    name=$(basename "${fixture}")
    in="${WORK}/${name}/in" out="${WORK}/${name}/out"
    mkdir -p "${in}" "${out}"
    cp -r "${fixture}." "${in}/"
    echo
    echo "════ ${name}"
    echo "── 1) k8s-installer-tui --render"
    (cd "${in}" && "${TUI}" --render "${out}") | sed 's|'"${out}"'/||; s/^/  /'
    [ "${PIPESTATUS[0]}" = 0 ] || { echo "  ✗ the tool's own validation failed"; fail=1; }

    echo "── 2) installers"
    python3 "${HERE}/check_installers.py" "${out}" "${GH}/k8s-installer" | sed 's/^/  /'
    [ "${PIPESTATUS[0]}" = 0 ] || fail=1

    if [ -f "${out}/files/aipub/vars.yml" ]; then
        echo "── 3) aipub charts with these site values (chart-images, site profile only)"
        mkdir -p "${WORK}/empty-profiles"
        OUT="${WORK}/${name}/charts" bash "${GH}/aipub-installer/tools/chart-images/check.sh" \
            "${out}/files/aipub/vars.yml" > "${WORK}/${name}/charts.log" 2>&1
        code=$?
        sed -n '/^════ site/,/^════ /p' "${WORK}/${name}/charts.log" | grep -E '^== |✗|✓|render' | sed 's/^/  /'
        # monitoring fails on aipub/busybox until the catalog carries it (AIP-3519); the
        # values are what is checked here, so only a render failure counts
        grep -q '✗ render' "${WORK}/${name}/charts.log" && { echo "  ✗ a chart did not render with these values"; fail=1; }
        [ "${code}" = 0 ] || grep -q 'aipub/busybox' "${WORK}/${name}/charts.log" || fail=1
    fi
    echo "── rendered files: ${out}"
    [ -n "${KEEP:-}" ] && cp -r "${out}" "${KEEP}/${name}"
done

echo
[ "${fail}" = 0 ] && echo "✓ every fixture passes" || echo "✗ a fixture fails; see above"
exit "${fail}"
