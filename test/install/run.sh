#!/usr/bin/env bash
# Drives `k8s-installer-tui --install` end to end against a fake installer root:
# a venv whose activate puts stub ansible, ansible-playbook and sudo first on
# PATH. The stubs log what they were run with and when, sleep a little, and
# fail on demand (FAKE_FAIL=<playbook name>). Nothing reaches a server.
#
#   docker run --rm -v tui-gomod:/go/pkg/mod -v <repo>:/src -w /src golang:1.23 bash test/install/run.sh
#
# Checks: the order and the one parallel stage (setup-cert beside the
# StorageClass stop) in the stubs' timestamps, the stop answered on a terminal
# (script, a pty), a failure and --install --resume running only from the
# failed step (with the copy again, and the options of the run it goes on
# with), state.json and the logs, the lock, Ctrl-C once and twice, --install
# refusing an unfinished run without --resume or --fresh, and a step left
# running by a program killed outright keeping the next start out.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "${HERE}/../.." && pwd)"
W=$(mktemp -d)
trap 'rm -rf "${W}"' EXIT
TUI="${W}/k8s-installer-tui"
ROOT="${W}/opt/k8s-installer"
STATE="${ROOT}/tui-install"
CALLS="${W}/calls.log"
fails=0
ok() { echo "  ✓ $*"; }
bad() { echo "  ✗ $*"; fails=$((fails + 1)); }

echo "── build"
(cd "${REPO}" && go build -o "${TUI}" .) || exit 1

# the installer root: release (P1), the ansible directory, a venv of stubs
mkdir -p "${ROOT}/ansible/group_vars/all" "${ROOT}/venv/bin" "${W}/stub"
echo "1.36.0" > "${ROOT}/release"
echo "export PATH=${W}/stub:\$PATH FAKE_DIR=${W}" > "${ROOT}/venv/bin/activate"

cat > "${W}/stub/ansible-playbook" <<'EOF'
#!/usr/bin/env bash
pb=$(basename "${3:-x}" .yml)
echo "$(date +%s.%N) start ${pb} $*" >> "${FAKE_DIR}/calls.log"
echo "PLAY [${pb}] ******"
echo "TASK [fake task of ${pb}] ******"
var="FAKE_SLEEP_${pb//-/_}"
sleep "${!var:-${FAKE_SLEEP:-0.3}}"
if [ "${FAKE_FAIL:-}" = "${pb}" ]; then
    echo "fatal: [master1]: FAILED! => {\"msg\": \"fake failure of ${pb}\"}"
    echo "$(date +%s.%N) end ${pb} 2" >> "${FAKE_DIR}/calls.log"
    exit 2
fi
echo "ok: [master1]"
echo "PLAY RECAP: master1 ok=1 failed=0"
echo "$(date +%s.%N) end ${pb} 0" >> "${FAKE_DIR}/calls.log"
EOF
# ansible: the checks (P2, P6 on every node) and the kubectl of the StorageClass
# stop, there once ${FAKE_DIR}/sc-ready exists
cat > "${W}/stub/ansible" <<'EOF'
#!/usr/bin/env bash
pattern=$1
args="$*"
case "${args}" in
*"get storageclass"*)
    echo "$(date +%s.%N) kubectl ${args##*get storageclass }" >> "${FAKE_DIR}/calls.log"
    class=$(echo "${args##*get storageclass }" | cut -d' ' -f1)
    if [ -f "${FAKE_DIR}/sc-ready" ]; then
        echo "${pattern} | CHANGED | rc=0 | (stdout) storageclass.storage.k8s.io/${class}"
    else
        echo "${pattern} | FAILED | rc=1 | (stdout)  (stderr) Error from server (NotFound): storageclasses.storage.k8s.io \"${class}\" not found"
        exit 2
    fi ;;
*)
    for n in master1 gpu1; do echo "${n} | CHANGED | rc=0 | (stdout) "; done ;;
esac
EOF
printf '#!/usr/bin/env bash\nwhile [ "${1#-}" != "$1" ]; do shift; done\nexec "$@"\n' > "${W}/stub/sudo"
chmod +x "${W}/stub/"*
# the stub sudo outside the venv too: the runner asks sudo -n true itself
# before the steps that run sudo here (C1, A5.1); FAKE_SUDO_FAILS makes it refuse
mkdir -p "${W}/bin"
cat > "${W}/bin/sudo" <<'EOF'
#!/usr/bin/env bash
if [ -n "${FAKE_SUDO_FAILS:-}" ]; then echo "sudo: a password is required" >&2; exit 1; fi
while [ "${1#-}" != "$1" ]; do shift; done
exec "$@"
EOF
chmod +x "${W}/bin/sudo"
export PATH="${W}/bin:${PATH}"

# the site: new-site-non-ha with HA on a StorageClass, saved outside the
# installer (the copy step C1 puts it there)
SITE="${W}/site"
cp -r "${REPO}/test/simulate/fixtures/new-site-non-ha" "${SITE}"
sed -i 's/^aipub_ha_mode: false/aipub_ha_mode: true\naipub_ha_mode_storage_class: "nfs-client"/; /^aipub_harbor_local_pv_node/d' "${SITE}/files/aipub/vars.yml"
cd "${SITE}" || exit 1
install_cmd="${TUI} --installer-root ${ROOT} --install"

echo "── 1) a run (--no-monitoring) that fails at setup-base, the StorageClass stop answered on a terminal"
mkfifo "${W}/keys"
(
    # Enter once the stop is shown, the class there by then
    for _ in $(seq 100); do grep -q '⏸ M1' "${W}/out1.txt" 2>/dev/null && break; sleep 0.1; done
    touch "${W}/sc-ready"
    printf '\r'
    sleep 20
) > "${W}/keys" &
feeder=$!
FAKE_FAIL=setup-base FAKE_SLEEP_setup_cert=3 script -qfec "${install_cmd} --no-monitoring" /dev/null < "${W}/keys" > "${W}/out1.txt" 2>&1
code=$?
kill "${feeder}" 2>/dev/null
sed 's/^/    /' "${W}/out1.txt"
[ "${code}" = 1 ] && ok "exit 1 on a failed step" || bad "exit ${code}, want 1"
grep -q '⏸ M1 스토리지클래스 드라이버를 설치하세요 — 위저드가 쓰는 클래스: nfs-client' "${W}/out1.txt" && ok "the stop says what to install" || bad "no stop prompt"
grep -q 'fake failure of setup-base' "${W}/out1.txt" && ok "the summary shows the last lines of the failed log" || bad "no log lines in the summary"
order=$(awk '$2=="start"{printf "%s ", $3}' "${CALLS}")
[ "${order}" = "setup-k8s-installer setup-control-node setup-cluster setup-network setup-cert setup-base " ] &&
    ok "order: ${order}" || bad "order: ${order}"
# the stop beside setup-cert: its kubectl checks start with setup-cert (the
# first may come a moment before it), the one after Enter while it runs, all
# before it ends; setup-base only after both
read -r cs ce < <(awk '$3=="setup-cert"{print $1}' "${CALLS}" | paste -sd' ')
ne=$(awk '$2=="end" && $3=="setup-network"{print $1}' "${CALLS}")
bs=$(awk '$2=="start" && $3=="setup-base"{print $1}' "${CALLS}")
checks=$(awk '$2=="kubectl"{print $1}' "${CALLS}" | paste -sd' ')
verdict=$(awk -v cs="${cs}" -v ce="${ce}" -v ne="${ne}" -v bs="${bs}" -v checks="${checks}" 'BEGIN{
    n = split(checks, c, " "); inside = 0; ok = (n >= 2 && bs > ce)
    for (i = 1; i <= n; i++) { if (c[i] < ne || c[i] > ce) ok = 0; if (c[i] > cs) inside++ }
    print (ok && inside >= 1) ? "yes" : "no" }')
[ "${verdict}" = yes ] && ok "M1 checked at ${checks} while setup-cert ran ${cs}..${ce}; setup-base at ${bs}" ||
    bad "M1 checks ${checks} vs setup-cert ${cs}..${ce}, network end ${ne}, base ${bs}"
grep -q '"id": "A3"' "${STATE}/state.json" && ok "state.json" || bad "no state.json"
python3 - "${STATE}/state.json" <<'EOF' && ok "state: C1..A2, M1 ok, K4 A4 skipped, A3 failed, the rest pending; monitoring off" || bad "state.json statuses"
import json, sys
run = json.load(open(sys.argv[1]))
st = {s["id"]: s["status"] for s in run["steps"]}
want = {"C1": "ok", "K1": "ok", "K3": "ok", "K4": "skipped", "A2": "ok", "M1": "ok", "A3": "failed", "A4": "skipped", "A5": "pending"}
bad = {k: (st.get(k), v) for k, v in want.items() if st.get(k) != v}
print("    statuses:", " ".join(f"{k}={v}" for k, v in st.items()), "monitoring:", run.get("monitoring"))
sys.exit(1 if bad or run.get("monitoring") is not False else 0)
EOF
for l in C1 K1 K3 A2 M1 A3; do [ -s "${STATE}/logs/${l}.log" ] || bad "no log ${l}.log"; done
ok "logs: $(ls "${STATE}/logs" | paste -sd' ')"
cmp -s "${SITE}/inventory.yml" "${ROOT}/ansible/inventory.yml" && ok "C1 copied the site into the installer" || bad "C1 did not copy"

echo "── 2) --install --resume after the site was saved again: the copy, then from the failed step only"
: > "${CALLS}"
echo "# saved again" >> "${SITE}/group_vars/all/vars.yml"
echo y | ${install_cmd} --resume > "${W}/out2.txt" 2>&1
code=$?
sed 's/^/    /' "${W}/out2.txt"
[ "${code}" = 0 ] && ok "exit 0" || bad "exit ${code}, want 0"
order=$(awk '$2=="start"{printf "%s ", $3}' "${CALLS}")
# the plan of the run it goes on with: no monitoring, though this start did not say --no-monitoring
case "${order}" in
"setup-base setup-aipub "*) ok "resumed without monitoring: ${order}" ;;
*) bad "resumed: ${order}" ;;
esac
grep -q '이어서 실행은 지난 실행의 설정으로 계획합니다 (이번 플래그와 다름): --no-monitoring=true' "${W}/out2.txt" &&
    ok "said the run's --no-monitoring is kept" || bad "no word of the kept --no-monitoring"
grep -q '✓ C1' "${W}/out2.txt" && grep -q '# saved again' "${ROOT}/ansible/group_vars/all/vars.yml" &&
    ok "C1 ran again: the installer has the site as saved again" || bad "the resume did not copy the site again"
grep -q '"outcome": "ok"' "${STATE}/state.json" && ok "state.json outcome ok" || bad "outcome"

echo "── 3) a site changed since: --resume asks first"
echo "# changed" >> "${SITE}/group_vars/all/vars.yml"
echo n | ${install_cmd} --resume > "${W}/out3.txt" 2>&1
code=$?
grep -q '사이트 파일이 바뀌었습니다' "${W}/out3.txt" && [ "${code}" = 2 ] && ok "asked, refused: exit 2" || bad "exit ${code}: $(cat "${W}/out3.txt")"

echo "── 4) the lock held by another run"
flock "${STATE}/lock" sleep 5 &
sleep 0.5
${install_cmd} < /dev/null > "${W}/out4.txt" 2>&1
code=$?
wait
grep -q '다른 k8s-installer-tui' "${W}/out4.txt" && [ "${code}" = 2 ] && ok "refused: exit 2" || bad "exit ${code}: $(tail -3 "${W}/out4.txt")"

echo "── 4b) sudo here wants a password: refused before C1, state.json as it was"
: > "${CALLS}"
before=$(md5sum < "${STATE}/state.json")
FAKE_SUDO_FAILS=1 ${install_cmd} < /dev/null > "${W}/out4b.txt" 2>&1
code=$?
[ "${code}" = 2 ] && grep -q 'C1 는 이 제어 노드에서 sudo 로 실행하는데, sudo 가 비밀번호를 묻습니다' "${W}/out4b.txt" &&
    [ "$(md5sum < "${STATE}/state.json")" = "${before}" ] && [ ! -s "${CALLS}" ] &&
    ok "refused: exit 2, nothing ran" || bad "exit ${code}: $(tail -3 "${W}/out4b.txt")"

echo "── 5) Ctrl-C once: the running step finishes, nothing after it"
: > "${CALLS}"
FAKE_SLEEP_setup_k8s_installer=3 ${install_cmd} < /dev/null > "${W}/out5.txt" 2>&1 &
pid=$!
for _ in $(seq 100); do grep -q 'start setup-k8s-installer' "${CALLS}" && break; sleep 0.1; done
kill -INT "${pid}"
wait "${pid}"
code=$?
order=$(awk '$2=="end"{printf "%s ", $3}' "${CALLS}")
[ "${code}" = 130 ] && [ "${order}" = "setup-k8s-installer " ] && ok "exit 130, ended: ${order}" || bad "exit ${code}, ended: ${order}"

echo "── 5b) --install over the stopped run, without --resume or --fresh: refused"
: > "${CALLS}"
${install_cmd} < /dev/null > "${W}/out5b.txt" 2>&1
code=$?
[ "${code}" = 2 ] && grep -q '이어서 하려면 --resume' "${W}/out5b.txt" && grep -q -- '--fresh' "${W}/out5b.txt" && [ ! -s "${CALLS}" ] &&
    ok "refused: exit 2, nothing ran" || bad "exit ${code}: $(cat "${W}/out5b.txt")"

echo "── 6) --fresh, Ctrl-C twice: the step is stopped now"
: > "${CALLS}"
start=$(date +%s)
FAKE_SLEEP_setup_k8s_installer=30 ${install_cmd} --fresh < /dev/null > "${W}/out6.txt" 2>&1 &
pid=$!
for _ in $(seq 100); do grep -q 'start setup-k8s-installer' "${CALLS}" && break; sleep 0.1; done
kill -INT "${pid}"; sleep 0.3; kill -INT "${pid}"
wait "${pid}"
code=$?
took=$(($(date +%s) - start))
grep -q '"status": "cancelled"' "${STATE}/state.json"
[ "${code}" = 130 ] && [ "${took}" -lt 20 ] && grep -q '"status": "cancelled"' "${STATE}/state.json" &&
    ok "exit 130 after ${took}s, K1 cancelled" || bad "exit ${code} after ${took}s"

echo "── 7) the program killed outright (SIGKILL): its step, left running, keeps the next start out"
: > "${CALLS}"
FAKE_SLEEP_setup_k8s_installer=30 ${install_cmd} --fresh < /dev/null > "${W}/out7.txt" 2>&1 &
pid=$!
for _ in $(seq 100); do grep -q 'start setup-k8s-installer' "${CALLS}" && break; sleep 0.1; done
sleep 0.3
kill -KILL "${pid}"
wait "${pid}" 2>/dev/null
pgid=$(python3 -c 'import json, sys; print(next((s.get("pgid", 0) for s in json.load(open(sys.argv[1]))["steps"] if s["id"] == "K1"), 0))' "${STATE}/state.json")
${install_cmd} --resume < /dev/null > "${W}/out7b.txt" 2>&1
code=$?
[ "${pgid}" -gt 0 ] && kill -0 -- "-${pgid}" 2>/dev/null && [ "${code}" = 2 ] && grep -q "pid ${pgid}" "${W}/out7b.txt" &&
    ok "refused while K1 (group ${pgid}) still runs: exit 2" || bad "pgid ${pgid}, exit ${code}: $(tail -3 "${W}/out7b.txt")"
[ "${pgid}" -gt 0 ] && kill -INT -- "-${pgid}" 2>/dev/null
for _ in $(seq 50); do kill -0 -- "-${pgid}" 2>/dev/null || break; sleep 0.1; done
: > "${CALLS}"
${install_cmd} --resume < /dev/null > "${W}/out7c.txt" 2>&1
code=$?
[ "${code}" = 0 ] && grep -q 'start setup-k8s-installer' "${CALLS}" && ok "once it ended: --resume ran from K1, exit 0" ||
    bad "exit ${code}: $(tail -3 "${W}/out7c.txt")"

echo
[ "${fails}" = 0 ] && echo "✓ --install end to end" || echo "✗ ${fails} check(s) failed"
exit "${fails}"
