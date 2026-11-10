#!/usr/bin/env bash
# The inside of run.ps1, as root in a throwaway ubuntu or rocky container: a
# control user (ops) runs k8s-installer-tui --check-ssh / --send-ssh-key against
# nodes that are users of the container's own sshd, one of them without sudo, one
# with another password, and one address nobody listens on.
#
# /tmp is a noexec tmpfs (run.ps1), as on hardened sites.
#
# ansible-inventory comes from ansible-core $ANSIBLE_CORE (2.17.* unless given) in a venv at the place setup.sh
# of k8s-installer makes it (/opt/k8s-installer/venv), which the tool sources.
set -Eeuo pipefail

if command -v apt-get &>/dev/null; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq >/dev/null
    apt-get install -y -qq openssh-server openssh-client sudo python3-venv bsdextrautils util-linux procps >/dev/null
    PYTHON=python3
else
    dnf install -y -q openssh-server openssh-clients sudo python3.11 util-linux passwd findutils procps-ng >/dev/null
    rm -f /run/nologin
    PYTHON=python3.11
fi
ssh -V
$PYTHON -m venv /opt/k8s-installer/venv
/opt/k8s-installer/venv/bin/pip install -q "ansible-core==${ANSIBLE_CORE:-2.17.*}" 2>&1 | grep -v 'new release of pip' || true
/opt/k8s-installer/venv/bin/ansible-inventory --version | head -1

# Spaces at both ends and inside, and characters a shell would expand
PASSWORD=' p@ss w0rd$1 `id` "q" '"'"'s'"'"' * '
OTHER_PASSWORD='another-one'

add_user() {
    useradd -m -s /bin/bash "$1"
    printf '%s:%s\n' "$1" "$2" | chpasswd
}
add_user ops "$PASSWORD"         # runs the tool
add_user alice "$PASSWORD"       # node with sudo
add_user bob "$PASSWORD"         # node without sudo
add_user carol "$OTHER_PASSWORD" # node with another password
add_user dave "$OTHER_PASSWORD"  # nodes the send is interrupted on
add_user erin "$PASSWORD"        # node whose sshd takes no password
for u in alice carol dave; do echo "$u ALL=(ALL) NOPASSWD:ALL" >"/etc/sudoers.d/$u"; done

mkdir -p /run/sshd
sed -i 's/^#\?PasswordAuthentication.*/PasswordAuthentication yes/' /etc/ssh/sshd_config
rm -f /etc/ssh/sshd_config.d/*.conf
ssh-keygen -A >/dev/null
/usr/sbin/sshd -p 22 -p 2222
# password login off: a key has to be put there by hand
/usr/sbin/sshd -p 2224 -o PasswordAuthentication=no -o PidFile=/run/sshd-2224.pid

# The installer tree as setup.sh leaves it, with configure-control-node-ssh.sh
# of k8s-installer
ANSIBLE=/opt/k8s-installer/ansible
mkdir -p "$ANSIBLE"
tr -d '\r' </src/configure-control-node-ssh.sh >"$ANSIBLE/configure-control-node-ssh.sh"
chmod +x "$ANSIBLE/configure-control-node-ssh.sh"
install -m 0755 /src/k8s-installer-tui /usr/local/bin/k8s-installer-tui

inventory() { cat >"$ANSIBLE/inventory.yml"; chown ops:ops "$ANSIBLE/inventory.yml"; }
inventory <<"EOF"
all:
  hosts:
    node-sudo:
      ansible_host: 127.0.0.1
      ansible_user: alice
    node-nosudo:
      ansible_host: 127.0.0.1
      ansible_port: 2222
      ansible_user: bob
    node-down:
      ansible_host: 127.0.0.1
      ansible_port: 2223
      ansible_user: alice
    node-nopw:
      ansible_host: 127.0.0.1
      ansible_port: 2224
      ansible_user: erin
    localhost:
      ansible_connection: local
EOF
chown -R ops:ops /opt/k8s-installer/ansible

failures=0

# run <name> <expected exit code> <feed> <arguments...>
# The tool gets a terminal of its own (script(1)), as ssh and the password prompt
# want one. feed is a function writing what is typed on it, after the prompt is up:
# ssh throws away what was typed ahead
run() {
    local name=$1 expected=$2 feed=$3
    shift 3
    local code=0
    $feed | su - ops -c "cd $ANSIBLE && LC_ALL=C.UTF-8 ${RUN_ENV:-} script -qec 'k8s-installer-tui $* --inventory inventory.yml' /dev/null" >/tmp/out 2>&1 || code=$?
    tr -d '\r' </tmp/out >/tmp/out.txt
    echo "=== $name (exit $code)"
    cat /tmp/out.txt
    if [[ $code -ne $expected ]]; then
        echo "!!! $name: expected exit $expected, got $code"
        failures=$((failures + 1))
    fi
}
nothing() { :; }
password() { sleep 8; printf '%s\n' "$PASSWORD"; }
other_password() { sleep 8; printf '%s\n' "$OTHER_PASSWORD"; }
# the password (wrong for dave), then Ctrl-C while the first node checks it
interrupt() { sleep 8; printf '%s\n' "$PASSWORD"; sleep 2; printf '\003'; sleep 1; }

# expect <name> <extended regular expression a line of the last output matches>
expect() {
    if ! grep -Eq -- "$2" /tmp/out.txt; then
        echo "!!! $1: no line matches /$2/"
        failures=$((failures + 1))
    fi
}

# reject <name> <text the last output must not have>
reject() {
    if grep -Fq -- "$2" /tmp/out.txt; then
        echo "!!! $1: the output has \"$2\""
        failures=$((failures + 1))
    fi
}

# the password nowhere in /tmp but the captured output (checked by reject), and
# no directory of the askpass helper left
clean_tmp() {
    if grep -rqF -- "$PASSWORD" /tmp --exclude=out --exclude=out.txt 2>/dev/null; then
        echo "!!! $1: the password is in /tmp"
        failures=$((failures + 1))
    fi
    if compgen -G '/tmp/k8s-installer-tui-ssh-*' >/dev/null; then
        echo "!!! $1: left $(echo /tmp/k8s-installer-tui-ssh-*)"
        failures=$((failures + 1))
    fi
    if [[ -n $(ls -A /home/ops/.cache/k8s-installer-tui 2>/dev/null) ]]; then
        echo "!!! $1: left $(ls -A /home/ops/.cache/k8s-installer-tui)"
        failures=$((failures + 1))
    fi
}

findmnt -no OPTIONS /tmp | grep -q noexec || { echo "!!! /tmp is not noexec"; exit 1; }
mkdir -p /tmp/xdg && chown ops:ops /tmp/xdg && chmod 700 /tmp/xdg # noexec as well

run "check before a key exists" 1 nothing --check-ssh
expect "check before a key exists" 'id_rsa 가 없습니다'
expect "check before a key exists" '^node-sudo +alice@127\.0\.0\.1 +거부 +- +.*Permission denied'
expect "check before a key exists" '^node-down +alice@127\.0\.0\.1:2223 +접속불가 +- +.*Connection refused'
reject "check before a key exists" 'localhost'
expect "check before a key exists" '^node-nopw +erin@127\.0\.0\.1:2224 +비밀번호로그인꺼짐 +- +.*키를 직접 넣어야 함'
[[ -e /home/ops/.ssh/id_rsa ]] && { echo "!!! check before a key exists: made a key"; failures=$((failures + 1)); }

# the helper can run neither in XDG_RUNTIME_DIR (noexec) nor in ~/.cache (a
# file there): the cause is said, nothing is sent
rm -rf /home/ops/.cache && su - ops -c 'touch ~/.cache'
RUN_ENV=XDG_RUNTIME_DIR=/tmp/xdg run "no place to run the helper" 2 password --send-ssh-key --ask-pass-once
expect "no place to run the helper" 'SSH_ASKPASS 도우미를 실행할 수 있는 디렉터리가 없어 키를 보내지 않았습니다 \(noexec'
reject "no place to run the helper" 'node-sudo:'
reject "no place to run the helper" "p@ss"
rm -f /home/ops/.cache
if [[ -s /home/alice/.ssh/authorized_keys ]]; then
    echo "!!! no place to run the helper: alice got a key"
    failures=$((failures + 1))
fi

# XDG_RUNTIME_DIR noexec: on to ~/.cache, with /tmp noexec too
RUN_ENV=XDG_RUNTIME_DIR=/tmp/xdg run "send with one password" 1 password --send-ssh-key --ask-pass-once
expect "send with one password" '비밀번호 \(화면에 보이지 않음\)'
expect "send with one password" '^node-sudo +alice@127\.0\.0\.1 +ok +ok$'
expect "send with one password" '^node-nosudo +bob@127\.0\.0\.1:2222 +ok +비밀번호필요 +.*NOPASSWD'
expect "send with one password" '^node-down +alice@127\.0\.0\.1:2223 +접속불가'
expect "send with one password" '^node-nopw +erin@127\.0\.0\.1:2224 +비밀번호로그인꺼짐'
expect "send with one password" '키 보내는 중 \(2대\)'
reject "send with one password" 'node-nopw:'
reject "send with one password" "$PASSWORD"
reject "send with one password" "p@ss"
clean_tmp "send with one password"
[[ -n $(ls -A /tmp/xdg) ]] && { echo "!!! send with one password: left in XDG_RUNTIME_DIR"; failures=$((failures + 1)); }

# id_rsa without id_rsa.pub: stop and say how to restore it; the key and the
# control node's authorized_keys stay as they are
mv /home/ops/.ssh/id_rsa.pub /home/ops/id_rsa.pub.keep
key_before=$(md5sum </home/ops/.ssh/id_rsa)
auth_before=$(md5sum </home/ops/.ssh/authorized_keys)
run "public key missing" 2 nothing --send-ssh-key
expect "public key missing" 'ssh-keygen -y -f /home/ops/\.ssh/id_rsa > /home/ops/\.ssh/id_rsa\.pub'
if [[ $(md5sum </home/ops/.ssh/id_rsa) != "$key_before" || -e /home/ops/.ssh/id_rsa.pub ||
    $(md5sum </home/ops/.ssh/authorized_keys) != "$auth_before" ]]; then
    echo "!!! public key missing: the key pair or authorized_keys changed"
    failures=$((failures + 1))
fi
mv /home/ops/id_rsa.pub.keep /home/ops/.ssh/id_rsa.pub

# --check-ssh leaves known_hosts as it is, a node not seen before included
su - ops -c "ssh-keygen -q -R '[127.0.0.1]:2222' >/dev/null"
known_hosts_before=$(md5sum </home/ops/.ssh/known_hosts)
run "check after" 1 nothing --check-ssh
expect "check after" '^node-sudo +alice@127\.0\.0\.1 +ok +ok$'
expect "check after" '^node-nosudo +bob@127\.0\.0\.1:2222 +ok +비밀번호필요'
expect "check after" '^node-down +alice@127\.0\.0\.1:2223 +접속불가 +- +.*Connection refused'
if [[ $(md5sum </home/ops/.ssh/known_hosts) != "$known_hosts_before" ]]; then
    echo "!!! check after: known_hosts changed"
    failures=$((failures + 1))
fi

# a host key that changed: the note says how to forget it
cp /home/ops/.ssh/known_hosts /tmp/known_hosts.keep
ssh-keygen -q -t ed25519 -N '' -f /tmp/fake_host_key
su - ops -c "ssh-keygen -q -R '[127.0.0.1]:2222' >/dev/null"
echo "[127.0.0.1]:2222 $(cut -d' ' -f1,2 /tmp/fake_host_key.pub)" >>/home/ops/.ssh/known_hosts
rm -f /tmp/fake_host_key /tmp/fake_host_key.pub
run "check with a changed host key" 1 nothing --check-ssh
expect "check with a changed host key" "^node-nosudo +bob@127\.0\.0\.1:2222 +호스트키변경 +- +.*ssh-keygen -R '\[127\.0\.0\.1\]:2222'"
cp /tmp/known_hosts.keep /home/ops/.ssh/known_hosts
rm -f /tmp/known_hosts.keep

# The key of the control node is what counts: a node that lets the user in with
# another key is not ready
su - ops -c "ssh-keygen -q -t ed25519 -N '' -f ~/.ssh/id_ed25519"
cat /home/ops/.ssh/id_ed25519.pub >/home/bob/.ssh/authorized_keys
run "check with another key only" 1 nothing --check-ssh
expect "check with another key only" '^node-nosudo +bob@127\.0\.0\.1:2222 +거부'
rm -f /home/ops/.ssh/id_ed25519 /home/ops/.ssh/id_ed25519.pub

# A node with another password: wrong with --ask-pass-once, typed at the prompt
# of ssh without it
inventory <<"EOF"
all:
  hosts:
    node-sudo:
      ansible_host: 127.0.0.1
      ansible_user: alice
    node-other:
      ansible_host: 127.0.0.1
      ansible_user: carol
EOF
run "one wrong password" 1 password --send-ssh-key --ask-pass-once
expect "one wrong password" '^node-sudo +alice@127\.0\.0\.1 +ok +ok$'
expect "one wrong password" '^node-other +carol@127\.0\.0\.1 +거부 +- +키 보내기 실패'
reject "one wrong password" "$PASSWORD"
reject "one wrong password" "p@ss"
clean_tmp "one wrong password"

run "prompt of ssh" 0 other_password --send-ssh-key
expect "prompt of ssh" 'node-other \(carol@127\.0\.0\.1, 포트 22\): 키를 보냅니다'
expect "prompt of ssh" '^node-other +carol@127\.0\.0\.1 +ok +ok$'
expect "prompt of ssh" '✓ 모든 노드가'
reject "prompt of ssh" "$OTHER_PASSWORD"

# with the key pair there, configure-control-node-ssh.sh does not run again (it
# would rewrite authorized_keys)
auth_mtime=$(stat -c %Y /home/ops/.ssh/authorized_keys)
sleep 1
run "again, nothing to do" 0 nothing --send-ssh-key
reject "again, nothing to do" '키를 보냅니다'
reject "again, nothing to do" '비밀번호 (화면에'
expect "again, nothing to do" '✓ 모든 노드가'
if [[ $(stat -c %Y /home/ops/.ssh/authorized_keys) != "$auth_mtime" ]]; then
    echo "!!! again, nothing to do: authorized_keys of the control node rewritten"
    failures=$((failures + 1))
fi

run "check, all ready" 0 nothing --check-ssh
expect "check, all ready" '^node-other +carol@127\.0\.0\.1 +ok +ok$'

# Ctrl-C while a node checks the password (made slow by PAM): exit 130, no
# node after it, nothing left behind
inventory <<"EOF"
all:
  hosts:
    dave-1:
      ansible_host: 127.0.0.1
      ansible_user: dave
    dave-2:
      ansible_host: 127.0.0.1
      ansible_port: 2222
      ansible_user: dave
EOF
sed -i '1i auth optional pam_exec.so quiet /bin/sleep 6' /etc/pam.d/sshd
run "interrupted" 130 interrupt --send-ssh-key --ask-pass-once
sed -i '1d' /etc/pam.d/sshd
expect "interrupted" '키 보내는 중 \(2대\)'
expect "interrupted" '중단했습니다'
reject "interrupted" 'dave-2:'
reject "interrupted" "$PASSWORD"
clean_tmp "interrupted"
# the whole session of ssh-copy-id was killed, the ssh under it included
if pgrep -u ops -x ssh >/dev/null; then
    echo "!!! interrupted: ssh left running: $(pgrep -u ops -a ssh)"
    failures=$((failures + 1))
fi
if [[ -s /home/dave/.ssh/authorized_keys ]]; then
    echo "!!! interrupted: dave got a key"
    failures=$((failures + 1))
fi

echo
if [[ $failures -ne 0 ]]; then
    echo "FAILED: $failures"
    exit 1
fi
echo "PASSED"
