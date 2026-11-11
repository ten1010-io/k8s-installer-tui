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
# refusing an unfinished run without --resume or --fresh, a step left running by
# a program killed outright keeping the next start out, and an existing cluster
# (the record k8s-installer leaves under /etc/k8s-installer, made here in the
# container) planned without setup-cluster, and a new site without the archive
# of the user registry: refused, run with --no-archive to a failure of
# setup-network on the images, then --resume --archive running K4 first. The
# new-site runs give a small fake archive. The stub ansible-playbook refuses a
# run whose k8s_cp_extra_files names a file that is not under files/, as
# k8s-installer's validate-hostvars.py does.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "${HERE}/../.." && pwd)"
W=$(mktemp -d)
# the record of scenario 8, removed whatever happens if this run made it
MADE_ETC=
trap 'rm -rf "${W}"; [ -n "${MADE_ETC}" ] && rm -rf /etc/k8s-installer' EXIT
TUI="${W}/k8s-installer-tui"
ROOT="${W}/opt/k8s-installer"
STATE="${ROOT}/tui-install"
CALLS="${W}/calls.log"
fails=0
# the checks below read the Korean messages: the program speaks English unless told
export K8S_INSTALLER_TUI_LANG=ko
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
# what validate-hostvars.py (validate_k8s_cp_extra_files) checks in every
# playbook that imports prepare-vars.yml, setup-k8s-installer aside
if [ "${pb}" != setup-k8s-installer ] && ! missing=$(python3 "${FAKE_DIR}/cpfiles.py" "$@"); then
    echo "fatal: [localhost]: FAILED! => {\"msg\": \"${missing}\"}"
    echo "$(date +%s.%N) end ${pb} 2" >> "${FAKE_DIR}/calls.log"
    exit 2
fi
var="FAKE_SLEEP_${pb//-/_}"
sleep "${!var:-${FAKE_SLEEP:-0.3}}"
if [ "${FAKE_FAIL:-}" = "${pb}" ]; then
    [ -n "${FAKE_SAY:-}" ] && echo "${FAKE_SAY}"
    echo "fatal: [master1]: FAILED! => {\"msg\": \"fake failure of ${pb}\"}"
    echo "$(date +%s.%N) end ${pb} 2" >> "${FAKE_DIR}/calls.log"
    exit 2
fi
echo "ok: [master1]"
echo "PLAY RECAP: master1 ok=1 failed=0"
echo "$(date +%s.%N) end ${pb} 0" >> "${FAKE_DIR}/calls.log"
EOF
# k8s_cp_extra_files as the run sees it (the last -e that sets it, else
# vars.yml) and whether each source is a file under files/ that is not empty
cat > "${W}/cpfiles.py" <<'EOF'
import json, os, re, sys
args, files = sys.argv[1:], None
for i in range(len(args) - 1):
    if args[i] == "-e" and args[i + 1].startswith("{"):
        extra = json.loads(args[i + 1])
        if "k8s_cp_extra_files" in extra:
            files = [f["src"] for f in extra["k8s_cp_extra_files"]]
if files is None:
    files, inside = [], False
    for line in open("group_vars/all/vars.yml"):
        if line.startswith("k8s_cp_extra_files:"):
            inside = True
            continue
        if inside and line.strip() and not line[0].isspace() and not line.startswith("-"):
            break
        m = re.match(r'\s*-?\s*src:\s*"?([^"\s]+)"?', line) if inside else None
        if m:
            files.append(m.group(1))
for f in files:
    path = os.path.join("files", f)
    if not os.path.isfile(path) or os.path.getsize(path) == 0:
        print(f"File[{f}] of the control node is not there")
        sys.exit(1)
EOF
# ansible: the checks (P2, P6 on every node), the kubectl of the StorageClass
# stop, there once ${FAKE_DIR}/sc-ready exists, and the slurp of the apiserver
# manifest P3 reads on an existing site (no flag put in by hand)
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
*"src=/etc/kubernetes/manifests/kube-apiserver.yaml"*)
    manifest=$(printf 'apiVersion: v1\nkind: Pod\nspec:\n  containers:\n  - name: kube-apiserver\n    command:\n    - kube-apiserver\n' | base64 -w0)
    echo "${pattern} | SUCCESS => {\"changed\": false, \"content\": \"${manifest}\", \"encoding\": \"base64\"}" ;;
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
# the archive of the package: a new AIPub site does not start without it
ARCHIVE="${W}/pkg/ki-cp-user-registry-artifacts.tgz"
mkdir -p "${W}/pkg" && echo "fake archive" > "${ARCHIVE}"
new_cmd="${install_cmd} --archive ${ARCHIVE}"

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
FAKE_FAIL=setup-base FAKE_SLEEP_setup_cert=3 script -qfec "${new_cmd} --no-monitoring" /dev/null < "${W}/keys" > "${W}/out1.txt" 2>&1
code=$?
kill "${feeder}" 2>/dev/null
sed 's/^/    /' "${W}/out1.txt"
[ "${code}" = 1 ] && ok "exit 1 on a failed step" || bad "exit ${code}, want 1"
grep -q '⏸ M1 스토리지클래스 드라이버를 설치하세요 — 위저드가 쓰는 클래스: nfs-client' "${W}/out1.txt" && ok "the stop says what to install" || bad "no stop prompt"
grep -q 'fake failure of setup-base' "${W}/out1.txt" && ok "the summary shows the last lines of the failed log" || bad "no log lines in the summary"
order=$(awk '$2=="start"{printf "%s ", $3}' "${CALLS}")
[ "${order}" = "setup-k8s-installer setup-control-node setup-cluster push-user-registry-artifacts setup-network setup-cert setup-base " ] &&
    ok "order: ${order}" || bad "order: ${order}"
grep -q "start push-user-registry-artifacts .*-e archive=${ARCHIVE}" "${CALLS}" && ok "K4 pushed the archive given" || bad "K4: $(grep push-user "${CALLS}")"
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
python3 - "${STATE}/state.json" <<'EOF' && ok "state: C1..A2, M1 ok, A4 skipped, A3 failed, the rest pending; monitoring off" || bad "state.json statuses"
import json, sys
run = json.load(open(sys.argv[1]))
st = {s["id"]: s["status"] for s in run["steps"]}
want = {"C1": "ok", "K1": "ok", "K3": "ok", "K4": "ok", "A2": "ok", "M1": "ok", "A3": "failed", "A4": "skipped", "A5": "pending"}
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
echo y | ${new_cmd} --resume > "${W}/out2.txt" 2>&1
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
echo n | ${new_cmd} --resume > "${W}/out3.txt" 2>&1
code=$?
grep -q '사이트 파일이 바뀌었습니다' "${W}/out3.txt" && [ "${code}" = 2 ] && ok "asked, refused: exit 2" || bad "exit ${code}: $(cat "${W}/out3.txt")"

echo "── 4) the lock held by another run"
flock "${STATE}/lock" sleep 5 &
sleep 0.5
${new_cmd} < /dev/null > "${W}/out4.txt" 2>&1
code=$?
wait
grep -q '다른 k8s-installer-tui' "${W}/out4.txt" && [ "${code}" = 2 ] && ok "refused: exit 2" || bad "exit ${code}: $(tail -3 "${W}/out4.txt")"

echo "── 4b) sudo here wants a password: refused before C1, state.json as it was"
: > "${CALLS}"
before=$(md5sum < "${STATE}/state.json")
FAKE_SUDO_FAILS=1 ${new_cmd} < /dev/null > "${W}/out4b.txt" 2>&1
code=$?
[ "${code}" = 2 ] && grep -q 'C1 는 이 제어 노드에서 sudo 로 실행하는데, sudo 가 비밀번호를 묻습니다' "${W}/out4b.txt" &&
    [ "$(md5sum < "${STATE}/state.json")" = "${before}" ] && [ ! -s "${CALLS}" ] &&
    ok "refused: exit 2, nothing ran" || bad "exit ${code}: $(tail -3 "${W}/out4b.txt")"

echo "── 5) Ctrl-C once: the running step finishes, nothing after it"
: > "${CALLS}"
FAKE_SLEEP_setup_k8s_installer=3 ${new_cmd} < /dev/null > "${W}/out5.txt" 2>&1 &
pid=$!
for _ in $(seq 100); do grep -q 'start setup-k8s-installer' "${CALLS}" && break; sleep 0.1; done
kill -INT "${pid}"
wait "${pid}"
code=$?
order=$(awk '$2=="end"{printf "%s ", $3}' "${CALLS}")
[ "${code}" = 130 ] && [ "${order}" = "setup-k8s-installer " ] && ok "exit 130, ended: ${order}" || bad "exit ${code}, ended: ${order}"

echo "── 5b) --install over the stopped run, without --resume or --fresh: refused"
: > "${CALLS}"
${new_cmd} < /dev/null > "${W}/out5b.txt" 2>&1
code=$?
[ "${code}" = 2 ] && grep -q '이어서 하려면 --resume' "${W}/out5b.txt" && grep -q -- '--fresh' "${W}/out5b.txt" && [ ! -s "${CALLS}" ] &&
    ok "refused: exit 2, nothing ran" || bad "exit ${code}: $(cat "${W}/out5b.txt")"

echo "── 6) --fresh, Ctrl-C twice: the step is stopped now"
: > "${CALLS}"
start=$(date +%s)
FAKE_SLEEP_setup_k8s_installer=30 ${new_cmd} --fresh < /dev/null > "${W}/out6.txt" 2>&1 &
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
FAKE_SLEEP_setup_k8s_installer=30 ${new_cmd} --fresh < /dev/null > "${W}/out7.txt" 2>&1 &
pid=$!
for _ in $(seq 100); do grep -q 'start setup-k8s-installer' "${CALLS}" && break; sleep 0.1; done
sleep 0.3
kill -KILL "${pid}"
wait "${pid}" 2>/dev/null
pgid=$(python3 -c 'import json, sys; print(next((s.get("pgid", 0) for s in json.load(open(sys.argv[1]))["steps"] if s["id"] == "K1"), 0))' "${STATE}/state.json")
${new_cmd} --resume < /dev/null > "${W}/out7b.txt" 2>&1
code=$?
[ "${pgid}" -gt 0 ] && kill -0 -- "-${pgid}" 2>/dev/null && [ "${code}" = 2 ] && grep -q "pid ${pgid}" "${W}/out7b.txt" &&
    ok "refused while K1 (group ${pgid}) still runs: exit 2" || bad "pgid ${pgid}, exit ${code}: $(tail -3 "${W}/out7b.txt")"
[ "${pgid}" -gt 0 ] && kill -INT -- "-${pgid}" 2>/dev/null
for _ in $(seq 50); do kill -0 -- "-${pgid}" 2>/dev/null || break; sleep 0.1; done
: > "${CALLS}"
${new_cmd} --resume < /dev/null > "${W}/out7c.txt" 2>&1
code=$?
[ "${code}" = 0 ] && grep -q 'start setup-k8s-installer' "${CALLS}" && ok "once it ended: --resume ran from K1, exit 0" ||
    bad "exit ${code}: $(tail -3 "${W}/out7c.txt")"

echo "── 8) an existing cluster (the record of k8s-installer on this node), AIPub installed now, JWKS on: no setup-cluster"
# the site with every control plane preset on, saved as the wizard would before
# setup-cert: the default audit policy written, the JWKS config waiting for its CA
SITE2="${W}/site2"
cp -r "${REPO}/test/simulate/fixtures/new-site-controlplane" "${SITE2}"
if [ -e /etc/k8s-installer ] || ! mkdir -p /etc/k8s-installer/certs 2>/dev/null; then
    bad "/etc/k8s-installer is there already or cannot be made: run this in the container"
else
    MADE_ETC=1
    rmdir /etc/k8s-installer/certs /etc/k8s-installer
    (cd "${SITE2}" && "${TUI}" --render "${SITE2}" > "${W}/out8r.txt" 2>&1)
    [ -s "${SITE2}/files/aipub/audit-policy.yaml" ] && [ ! -e "${SITE2}/files/aipub/authentication-config.yaml" ] &&
        ok "saved before setup-cert: audit policy written, no JWKS config" || bad "render: $(cat "${W}/out8r.txt")"
    # setup-cluster has run here once; setup-cert will have put the CA by A5.1
    mkdir -p /etc/k8s-installer/certs
    echo "1.36.0" > /etc/k8s-installer/applied-release
    cp "${SITE2}/jwks-ca.crt" /etc/k8s-installer/certs/ca.crt
    : > "${CALLS}"
    (cd "${SITE2}" && ${install_cmd} --fresh < /dev/null > "${W}/out8.txt" 2>&1)
    code=$?
    sed 's/^/    /' "${W}/out8.txt"
    [ "${code}" = 0 ] && ok "exit 0: every playbook found the files its k8s_cp_extra_files names" || bad "exit ${code}, want 0"
    grep -q '── 사이트: 기존 사이트: /etc/k8s-installer/applied-release 있음' "${W}/out8.txt" &&
        ok "said why it is an existing site" || bad "no site line"
    order=$(awk '$2=="start"{printf "%s ", $3}' "${CALLS}")
    [ "${order}" = "update-cluster setup-network setup-cert update-cluster setup-base setup-monitoring setup-aipub update-cluster " ] &&
        ok "order: ${order}" || bad "order: ${order}"
    k3=$(awk '$2=="start" && $3=="update-cluster"{print; exit}' "${CALLS}")
    case "${k3}" in
    *audit-policy-file*authentication-config*) bad "K3 applies JWKS before its CA: ${k3}" ;;
    *audit-policy-file*) ok "K3 is update-cluster with the cluster stage (audit log, no JWKS)" ;;
    *) bad "K3: ${k3}" ;;
    esac
    python3 - "${STATE}/state.json" <<'EOF' && ok "state: K1 K2 skipped, K3 ok" || bad "state.json statuses"
import json, sys
st = {s["id"]: s["status"] for s in json.load(open(sys.argv[1]))["steps"]}
print("    statuses:", " ".join(f"{k}={v}" for k, v in st.items()))
sys.exit(0 if (st.get("K1"), st.get("K2"), st.get("K3"), st.get("A5.2")) == ("skipped", "skipped", "ok", "ok") else 1)
EOF
    # the stub does refuse what k8s-installer refuses: vars.yml whole, no JWKS config
    rm -f "${ROOT}/ansible/files/aipub/authentication-config.yaml"
    (cd "${ROOT}/ansible" && FAKE_DIR="${W}" "${W}/stub/ansible-playbook" -i inventory.yml playbooks/setup-control-node.yml > "${W}/out8b.txt" 2>&1)
    code=$?
    [ "${code}" = 2 ] && grep -q 'File\[aipub/authentication-config.yaml\] of the control node is not there' "${W}/out8b.txt" &&
        ok "the stub refuses a run that names a missing file" || bad "stub: exit ${code}: $(cat "${W}/out8b.txt")"
fi

echo "── 9) a new site without the archive: refused; with --no-archive K4 skipped and A1 fails on the images; --resume --archive runs K4, then A1"
# a new site again: the record of scenario 8 gone, as reset-cluster.yml takes it
[ -n "${MADE_ETC}" ] && rm -f /etc/k8s-installer/applied-release
# the installer holds site2 since scenario 8: back to the site as C1 last copied
# it, or the copy stops on what looks like an edit made in the installer
cp "${SITE}/inventory.yml" "${ROOT}/ansible/inventory.yml"
cp "${SITE}/group_vars/all/vars.yml" "${ROOT}/ansible/group_vars/all/vars.yml"
cp "${SITE}/files/aipub/vars.yml" "${ROOT}/ansible/files/aipub/vars.yml"
if [ -e /etc/k8s-installer/applied-release ]; then
    bad "/etc/k8s-installer/applied-release is there: run this in the container"
else
    : > "${CALLS}"
    # HOME=${W}: the package unpacked as ~/pkg, where the refusal finds the archive
    HOME="${W}" ${install_cmd} --fresh < /dev/null > "${W}/out9a.txt" 2>&1
    code=$?
    [ "${code}" = 2 ] && grep -q '새 사이트인데 --archive 가 없습니다' "${W}/out9a.txt" && grep -q -- '--no-archive' "${W}/out9a.txt" &&
        grep -q "찾은 파일: ${ARCHIVE}" "${W}/out9a.txt" && [ ! -s "${CALLS}" ] &&
        ok "no archive: exit 2, said why and where it is, nothing ran" || bad "exit ${code}: $(cat "${W}/out9a.txt")"
    ${install_cmd} --fresh --archive "${W}/pkg" < /dev/null > "${W}/out9b.txt" 2>&1
    code=$?
    [ "${code}" = 2 ] && grep -q '일반 파일이 아닙니다' "${W}/out9b.txt" && [ ! -s "${CALLS}" ] &&
        ok "an archive that is no file: exit 2, nothing ran" || bad "exit ${code}: $(cat "${W}/out9b.txt")"

    FAKE_FAIL=setup-network FAKE_SAY="Error: INSTALLATION FAILED: context deadline exceeded" \
        ${install_cmd} --fresh --no-archive < /dev/null > "${W}/out9c.txt" 2>&1
    code=$?
    sed 's/^/    /' "${W}/out9c.txt"
    order=$(awk '$2=="start"{printf "%s ", $3}' "${CALLS}")
    [ "${code}" = 1 ] && [ "${order}" = "setup-k8s-installer setup-control-node setup-cluster setup-network " ] &&
        ok "--no-archive: K4 skipped, A1 failed: ${order}" || bad "exit ${code}, order: ${order}"
    grep -q 'kubectl -n kube-system get pods' "${W}/out9c.txt" && grep -q 'K4 push-user-registry-artifacts 를 건너뛰었습니다' "${W}/out9c.txt" &&
        ok "the note points at the images and the skipped K4" || bad "no diagnosis in the summary"

    : > "${CALLS}"
    ${install_cmd} --resume < /dev/null > "${W}/out9d.txt" 2>&1
    code=$?
    [ "${code}" = 2 ] && grep -q '새 사이트인데 --archive 가 없습니다' "${W}/out9d.txt" && [ ! -s "${CALLS}" ] &&
        ok "--resume with K4 not done and no archive: exit 2" || bad "exit ${code}: $(cat "${W}/out9d.txt")"
    ${install_cmd} --resume --archive "${ARCHIVE}" < /dev/null > "${W}/out9e.txt" 2>&1
    code=$?
    sed 's/^/    /' "${W}/out9e.txt"
    order=$(awk '$2=="start"{printf "%s ", $3}' "${CALLS}")
    case "${order}" in
    "push-user-registry-artifacts setup-network setup-cert "*) [ "${code}" = 0 ] && ok "resumed: ${order}" || bad "exit ${code}" ;;
    *) bad "exit ${code}, resumed: ${order}" ;;
    esac
    grep -q "start push-user-registry-artifacts .*-e archive=${ARCHIVE}" "${CALLS}" && ok "K4 pushed the archive of this start" || bad "K4 args"
    python3 - "${STATE}/state.json" "${ARCHIVE}" <<'EOF' && ok "state.json: K4 ok, archive recorded" || bad "state.json"
import json, sys
run = json.load(open(sys.argv[1]))
st = {s["id"]: s["status"] for s in run["steps"]}
sys.exit(0 if st.get("K4") == "ok" and run.get("archive_path") == sys.argv[2] and run.get("outcome") == "ok" else 1)
EOF
fi

echo
[ "${fails}" = 0 ] && echo "✓ --install end to end" || echo "✗ ${fails} check(s) failed"
exit "${fails}"
