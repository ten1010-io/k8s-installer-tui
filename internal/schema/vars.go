// Code below was first generated from the comments of the embedded templates
// and the ki_var_classes table of k8s-installer; it is maintained by hand now.
// schema_test.go checks Long and Default against the templates again.

package schema

var vars = []Var{
	{
		Key:       "ansible_host",
		File:      FileInventory,
		Group:     "nodes",
		Type:      TypeString,
		Host:      true,
		Required:  Always,
		Short:     "노드 접속 주소 (IP)",
		Preflight: "control node 에서 이 주소로 ssh 접속이 되어야 함",
		Screen:    "1.노드",
	},
	{
		Key:     "ansible_port",
		File:    FileInventory,
		Group:   "nodes",
		Type:    TypePort,
		Host:    true,
		Default: "22",
		Short:   "SSH 포트 (비우면 22)",
		Screen:  "1.노드",
	},
	{
		Key:    "ansible_user",
		File:   FileInventory,
		Group:  "nodes",
		Type:   TypeString,
		Host:   true,
		Short:  "이 노드만 다른 로그인 사용자 (비우면 vars.yml 의 ansible_user)",
		Screen: "1.노드",
	},
	{
		Key:     "ki_cp_node",
		File:    FileInventory,
		Group:   "nodes",
		Type:    TypeList,
		Default: "[]",
		Short:   "ki control plane 노드 (DNS·NTP·레지스트리·LB 를 돌림)",
		Screen:  "2.그룹",
	},
	{
		Key:      "k8s_node",
		File:     FileInventory,
		Group:    "nodes",
		Type:     TypeList,
		Default:  "[]",
		Required: Always,
		Short:    "쿠버네티스 노드 전체",
		Screen:   "2.그룹",
	},
	{
		Key:      "k8s_cp",
		File:     FileInventory,
		Group:    "nodes",
		Type:     TypeBool,
		Host:     true,
		Default:  "false",
		Short:    "k8s 컨트롤 플레인(마스터) 노드",
		VarClass: "node_rebuild",
		Screen:   "2.그룹",
	},
	{
		Key:      "k8s_node_labels",
		File:     FileInventory,
		Group:    "nodes",
		Type:     TypeMap,
		Host:     true,
		Default:  "{}",
		Short:    "노드 라벨 (AIPub CP 노드에 node-role.aipub.ten1010.io/control-plane)",
		VarClass: "node_metadata",
		Screen:   "5.AIPub",
	},
	{
		Key:     "broken_node",
		File:    FileInventory,
		Group:   "nodes",
		Type:    TypeList,
		Default: "[]",
		Short:   "고장 나 작업에서 빼 둔 노드",
	},
	{
		Key:      "control_node_ih",
		File:     FileInventory,
		Group:    "nodes",
		Type:     TypeString,
		Host:     true,
		Short:    "control node 가 될 ki cp 노드 (첫 번째 ki cp 노드)",
		VarClass: "no_action",
		Screen:   "2.그룹",
	},
	{
		Key:     "ansible_connection",
		File:    FileInventory,
		Group:   "nodes",
		Type:    TypeString,
		Host:    true,
		Default: "local",
		Short:   "control node(localhost) 는 로컬로 접속",
	},
	{
		Key:       "ansible_user",
		File:      FileK8s,
		Group:     "nodes",
		Type:      TypeString,
		Default:   "root",
		Short:     "모든 노드에 로그인하는 사용자 (기본 root)",
		Preflight: "모든 노드에서 `sudo -n /bin/sh -c true` 가 통과해야 함 (root 가 아닐 때)",
		Long: `The user ansible logs in to every node as, the control node included. root
works as it always has. Any other user has to exist on every node under this
one name, hold the public key configure-control-node-ssh.sh prints, and be
allowed sudo without a password - the playbooks run every script through sudo
and nothing is there to type one. The user ends up with what root has, so this
is for sites whose policy forbids logging in as root rather than for giving the
installer less. To log in to one node as someone else, set it on that node in
inventory.yml`,
	},
	{
		Key:      "ki_var_root_path",
		File:     FileK8s,
		Group:    "host",
		Type:     TypeString,
		Default:  "/var/lib/k8s-installer",
		Short:    "설치기 상태 디렉터리",
		VarClass: "node_rebuild",
	},
	{
		Key:      "docker_root_path",
		File:     FileK8s,
		Group:    "host",
		Type:     TypeString,
		Default:  "/var/lib/docker",
		Short:    "docker 데이터 디렉터리",
		VarClass: "node_rebuild",
	},
	{
		Key:      "ephemeral_storage_device",
		File:     FileK8s,
		Group:    "host",
		Type:     TypeString,
		Short:    "kubelet 임시 저장소용 빈 블록 장치 (null=루트 파일시스템)",
		VarClass: "node_rebuild",
		Long: `Block device holding everything kubelet accounts as ephemeral storage, so that
an image or a pod log can not fill the filesystem the operating system runs on.
Leave it null to keep those paths on the root filesystem
To give a single node a device, set this variable for that node in inventory.yml

The device is formatted, so it must be one with nothing on it, and it is only
applied while a node is being provisioned. An existing node is taken out with
remove-node and added back with add-node

Do not point this at a disk whose contents do not survive a reboot. The name
says ephemeral, but /var/lib/kubelet also holds state kubelet expects to find
again after a restart. See docs/impl-notes.adoc`,
	},
	{
		Key:      "kernel_cmdline_extra_args",
		File:     FileK8s,
		Group:    "host",
		Type:     TypeList,
		Default:  "[]",
		Short:    "추가 커널 부팅 인자 (노드별로 inventory.yml 에)",
		VarClass: "node_rebuild",
		Long: `Kernel command line arguments this node boots with, on top of what the
distribution puts there. Set per node in inventory.yml, since what a node has
to boot with follows from what it holds.

  kernel_cmdline_extra_args:
    - "default_hugepagesz=1G"
    - "hugepagesz=1G"
    - "hugepages=64"
    - "isolcpus=8-63"

Hugepages reserved at boot, cpus isolated from the scheduler, a driver
parameter: everything a node has to be told before userspace starts. The iommu
arguments are not here - the vfio-pci component derives those from the devices
named for it and owns them, and naming one here is refused.

Empty leaves the node alone, and emptying a list that had something in it takes
what it had away. Either way the command line changes at the next boot:
setup-boot-config.yml is what reboots a node while it is being built, and a
node that was not rebooted is reported at the end of every run until it is`,
	},
	{
		Key:      "kernel_modules_extra",
		File:     FileK8s,
		Group:    "host",
		Type:     TypeList,
		Default:  "[]",
		Short:    "추가로 올릴 커널 모듈",
		VarClass: "node_kernel",
		Long: `Kernel modules this node loads, and sysctls it is set to. Set per node in
inventory.yml, since what a node needs follows from what runs on it.

  kernel_modules_extra:
    - "openvswitch"
    - "geneve"
  sysctl_extra:
    net.ipv4.conf.all.rp_filter: 0

What this is for is what a cni asks of a node. This installer brings kubernetes
up and not the network on top of it, so what kubernetes itself needs is already
written by the node setup and what the layer above needs had nowhere to be
declared. Left to be run by hand, those are what a node quietly lacks after it
has been rebuilt.

In force as soon as the playbook has run - the module is loaded and the sysctl
is set now, and the files are what bring them back at the next boot. So no
reboot is asked for, unlike the kernel command line.

The five sysctls and the two modules this installer sets for kubernetes are
refused here. /etc/sysctl.d is read in the order its file names sort in and the
last value read is the one that holds, and the k8s.conf the node setup writes
sorts after the 99-ki-sysctl.conf these go into - "9" before "k" - so a key
claimed by both is one where what is written here loses without saying so.

Emptying a list takes the file away, and with it what the node would have
loaded or set at its next boot. What is already loaded stays loaded and what is
already set stays set: a module in use can not be removed, and the value a
sysctl held before it was set is written down nowhere`,
	},
	{
		Key:      "sysctl_extra",
		File:     FileK8s,
		Group:    "host",
		Type:     TypeMap,
		Default:  "{}",
		Short:    "추가 sysctl",
		VarClass: "node_kernel",
		Long: `Kernel modules this node loads, and sysctls it is set to. Set per node in
inventory.yml, since what a node needs follows from what runs on it.

  kernel_modules_extra:
    - "openvswitch"
    - "geneve"
  sysctl_extra:
    net.ipv4.conf.all.rp_filter: 0

What this is for is what a cni asks of a node. This installer brings kubernetes
up and not the network on top of it, so what kubernetes itself needs is already
written by the node setup and what the layer above needs had nowhere to be
declared. Left to be run by hand, those are what a node quietly lacks after it
has been rebuilt.

In force as soon as the playbook has run - the module is loaded and the sysctl
is set now, and the files are what bring them back at the next boot. So no
reboot is asked for, unlike the kernel command line.

The five sysctls and the two modules this installer sets for kubernetes are
refused here. /etc/sysctl.d is read in the order its file names sort in and the
last value read is the one that holds, and the k8s.conf the node setup writes
sorts after the 99-ki-sysctl.conf these go into - "9" before "k" - so a key
claimed by both is one where what is written here loses without saying so.

Emptying a list takes the file away, and with it what the node would have
loaded or set at its next boot. What is already loaded stays loaded and what is
already set stays set: a module in use can not be removed, and the value a
sysctl held before it was set is written down nowhere`,
	},
	{
		Key:      "internal_network_subnets",
		File:     FileK8s,
		Group:    "network",
		Type:     TypeList,
		Elem:     TypeCIDR,
		Default:  `["192.168.0.0/24"]`,
		Required: Always,
		Short:    "노드와 ki-cp VIP 가 함께 드는 대역 (실제 넷마스크로)",
		VarClass: "immutable",
		Screen:   "3.네트워크",
	},
	{
		Key:      "internal_network_extra_zone",
		File:     FileK8s,
		Group:    "network",
		Type:     TypeFQDN,
		Short:    "내부 DNS 가 답하는 추가 존 (AIPub 도메인)",
		VarClass: "service",
		Screen:   "3.네트워크",
	},
	{
		Key:      "internal_network_extra_zone_a_records",
		File:     FileK8s,
		Group:    "network",
		Type:     TypeList,
		Default:  "[]",
		Short:    "추가 존의 A 레코드 (@, * → 인그레스 VIP)",
		VarClass: "service",
		Screen:   "3.네트워크",
	},
	{
		Key:      "ki_cp_ha_mode",
		File:     FileK8s,
		Group:    "network",
		Type:     TypeBool,
		Default:  "true",
		Short:    "ki control plane HA (VIP 사용)",
		VarClass: "immutable",
		Screen:   "3.네트워크",
	},
	{
		Key:       "ki_cp_ha_mode_vip",
		File:      FileK8s,
		Group:     "network",
		Type:      TypeIP,
		Required:  "ki_cp_ha_mode=true",
		Short:     "ki-cp VIP",
		Preflight: "internal_network_subnets 안의 비어 있는 IP 여야 arp 로 잡힘",
		VarClass:  "service",
		Screen:    "3.네트워크",
	},
	{
		Key:      "ki_cp_ha_mode_vip_carrier",
		File:     FileK8s,
		Group:    "network",
		Type:     TypeEnum,
		Default:  "arp",
		Enum:     []string{"arp", "aws-eni"},
		Short:    "VIP 를 알리는 방식",
		VarClass: "service",
		Long: `Who tells the network where the vip is.

arp is a node announcing the address itself, with gratuitous arp, as soon as
keepalived puts it on an interface. That is what a switch listens to and what
a cloud ignores: a vpc delivers to the addresses it has on record for a
network interface and to nothing else, so on ec2 the vip goes up on a node and
no packet ever arrives.

aws-eni is for ec2. The node keepalived makes MASTER takes the vip onto its
own network interface as a secondary private address, through the ec2 api,
and the node it was on loses it in the same call. Everything else - who is
elected, when the address moves, that a node which came back does not take
it back - is the keepalived above either way. What it needs from the site is
in README.adoc: an instance profile allowing ec2:AssignPrivateIpAddresses on
every ki cp node, a way to reach the ec2 api, and the vip kept free in the
subnet of the ki cp nodes`,
	},
	{
		Key:      "ki_cp_service_timezone",
		File:     FileK8s,
		Group:    "network",
		Type:     TypeString,
		Short:    "ki cp 서비스 로그 시간대 (null=노드 시간대)",
		VarClass: "service",
		Long: `The time zone the ki cp services render their timestamps in. Null takes the
zone of the node each one runs on, which is what makes the log of a container
read like the journal of the machine it is on.

Set it to an iana name to override that for every ki cp node at once, which is
what a site wants when its machines are on local time and its operators read
everything in UTC.

Only the dns server and the load balancer render in it. The registries and
keepalived carry no tzdata, and chrony writes its own log in utc whatever it
is told, so those four are in utc either way. See README.adoc`,
	},
	{
		Key:      "ki_cp_dns_dnssec_validation",
		File:     FileK8s,
		Group:    "network",
		Type:     TypeBool,
		Default:  "true",
		Short:    "DNSSEC 검증",
		VarClass: "service",
		Screen:   "3.네트워크",
		Long: `The time zone the ki cp services render their timestamps in. Null takes the
zone of the node each one runs on, which is what makes the log of a container
read like the journal of the machine it is on.

Set it to an iana name to override that for every ki cp node at once, which is
what a site wants when its machines are on local time and its operators read
everything in UTC.

Only the dns server and the load balancer render in it. The registries and
keepalived carry no tzdata, and chrony writes its own log in utc whatever it
is told, so those four are in utc either way. See README.adoc`,
	},
	{
		Key:      "ki_cp_dns_server_upstream_servers",
		File:     FileK8s,
		Group:    "network",
		Type:     TypeList,
		Elem:     TypeIP,
		Default:  `["8.8.8.8", "8.8.4.4"]`,
		Required: Always,
		Short:    "ki cp DNS 의 상위 DNS 서버",
		VarClass: "service",
		Screen:   "3.네트워크",
		Long: `The time zone the ki cp services render their timestamps in. Null takes the
zone of the node each one runs on, which is what makes the log of a container
read like the journal of the machine it is on.

Set it to an iana name to override that for every ki cp node at once, which is
what a site wants when its machines are on local time and its operators read
everything in UTC.

Only the dns server and the load balancer render in it. The registries and
keepalived carry no tzdata, and chrony writes its own log in utc whatever it
is told, so those four are in utc either way. See README.adoc`,
	},
	{
		Key:      "ki_cp_ntp_server_upstream_servers",
		File:     FileK8s,
		Group:    "network",
		Type:     TypeList,
		Default:  `["time1.google.com", "time2.google.com"]`,
		Required: Always,
		Short:    "ki cp NTP 의 상위 NTP 서버",
		VarClass: "service",
		Screen:   "3.네트워크",
		Long: `The time zone the ki cp services render their timestamps in. Null takes the
zone of the node each one runs on, which is what makes the log of a container
read like the journal of the machine it is on.

Set it to an iana name to override that for every ki cp node at once, which is
what a site wants when its machines are on local time and its operators read
everything in UTC.

Only the dns server and the load balancer render in it. The registries and
keepalived carry no tzdata, and chrony writes its own log in utc whatever it
is told, so those four are in utc either way. See README.adoc`,
	},
	{
		Key:      "ki_cp_ntp_server_local_fallback",
		File:     FileK8s,
		Group:    "network",
		Type:     TypeBool,
		Default:  "true",
		Short:    "상위 NTP 가 끊기면 ki cp 시계를 나눠줌",
		VarClass: "service",
		Long: `Whether the ntp server of a ki cp node hands out its own clock when none of
the upstream servers above can be reached. On, the nodes keep following the
ki cp through an outage of its upstream, agreeing with one another on a time
that is only as right as its clock, and go back to the upstream when it
returns. Off, a ki cp whose upstream is gone hands out nothing, the nodes
stop synchronising and the clock check of check-node-state.sh refuses every
playbook, which is the choice of a site that would rather stop than run on a
clock nothing checks. An empty list above is the ki cp clock as the only time
there is, whatever this says. See README.adoc`,
	},
	{
		Key:       "k8s_pod_subnet",
		File:      FileK8s,
		Group:     "kubernetes",
		Type:      TypeCIDR,
		Default:   "10.244.0.0/16",
		Required:  Always,
		Short:     "파드 대역 (CNI 에도 같은 값)",
		Preflight: "노드·서비스가 쓰는 망과 겹치지 않아야 함",
		VarClass:  "immutable",
		Screen:    "4.K8s",
		Long: `The addresses kubernetes hands out inside the cluster. Fixed while a cluster
exists: the service subnet decides the address of every service, including the
one kubelet is told is the cluster dns, and the pod subnet is what the cni was
installed around. Moving either is a cluster rebuilt rather than a value
changed, and update-cluster.yml refuses it.

They are here because they can collide with a network that was there first.
Every node, every service the workloads reach and every address the cluster
routes to has to sit outside both, so a site already using 10.244 or 10.96 for
something needs them somewhere else, and there is nothing this installer could
pick that is right everywhere.

Nothing installs a cni here, so whatever does has to be given these same two
values. A cni brought up around a different pod subnet leaves nodes that never
reach Ready, and the reason does not show up where the symptom does`,
	},
	{
		Key:      "k8s_service_subnet",
		File:     FileK8s,
		Group:    "kubernetes",
		Type:     TypeCIDR,
		Default:  "10.96.0.0/12",
		Required: Always,
		Short:    "서비스 대역",
		VarClass: "immutable",
		Screen:   "4.K8s",
		Long: `The addresses kubernetes hands out inside the cluster. Fixed while a cluster
exists: the service subnet decides the address of every service, including the
one kubelet is told is the cluster dns, and the pod subnet is what the cni was
installed around. Moving either is a cluster rebuilt rather than a value
changed, and update-cluster.yml refuses it.

They are here because they can collide with a network that was there first.
Every node, every service the workloads reach and every address the cluster
routes to has to sit outside both, so a site already using 10.244 or 10.96 for
something needs them somewhere else, and there is nothing this installer could
pick that is right everywhere.

Nothing installs a cni here, so whatever does has to be given these same two
values. A cni brought up around a different pod subnet leaves nodes that never
reach Ready, and the reason does not show up where the symptom does`,
	},
	{
		Key:       "k8s_kube_proxy_enabled",
		File:      FileK8s,
		Group:     "kubernetes",
		Type:      TypeBool,
		Default:   "true",
		Short:     "kube-proxy 설치 (AIPub 이면 false — cilium 이 대체)",
		Preflight: "클러스터가 이미 있으면 바꿀 수 없음",
		VarClass:  "immutable",
		Screen:    "5.AIPub",
		Long: `Whether kubeadm installs kube-proxy. Set to false for a cni that routes the
services itself - cilium with kubeProxyReplacement, calico in ebpf mode - and
kubeadm makes no kube-proxy DaemonSet, now or at any upgrade after. Nothing
else here changes: the cni is still what brings the pod network up, and until
it does no service address is reachable, kube-proxy or not.

A cni that replaces kube-proxy has to be told where the apiserver is, since
the kubernetes service address it would otherwise use is one of the addresses
it is now in charge of routing. The address to give it is the one every
kubelet already uses, ki-cp.<internal_network_zone>:<ki_cp_k8s_cp_lb_port>,
which is ki-cp.ki.local:16443 unless constant-vars.yml says otherwise.

Fixed while a cluster exists, like the subnets: taking kube-proxy out of a
running cluster is a DaemonSet deleted and the rules it wrote cleaned off
every node, and putting it back is an addon phase run again, and neither is
what update-cluster.yml does. It refuses the change instead`,
	},
	{
		Key:      "k8s_minor_version",
		File:     FileK8s,
		Group:    "kubernetes",
		Type:     TypeMinor,
		Default:  "1.36",
		Required: Always,
		Short:    "쿠버네티스 마이너 버전 (릴리스가 지원하는 것)",
		VarClass: "cluster_upgrade",
		Screen:   "4.K8s",
		Long: `The kubernetes minor this cluster runs. It has to be one the release carries:
release.yml lists them under k8s_versions, and the patch of that minor is the
release to pin rather than this file to choose.

Left where it is, a release upgrade keeps the cluster on this minor and moves
it only to the patch the new release pins. Raised by one, upgrade-cluster.yml
takes the cluster up a minor, which is as far as kubeadm goes at a time`,
	},
	{
		Key:      "k8s_apiserver_extra_args",
		File:     FileK8s,
		Group:    "control_plane",
		Type:     TypeList,
		Default:  "[]",
		Short:    "kube-apiserver 추가 인자",
		VarClass: "control_plane",
		Long: `Flags and host paths added to the kube-apiserver of every control plane node.
Empty leaves it exactly as kubeadm builds it.

What this is for is the apiserver options a cluster is required to run with and
the installer has no opinion about: an audit policy, an oidc issuer, a feature
gate, a runtime-config. Without it the only way to set one is to edit the static
pod manifest by hand on each node, which the next kubeadm upgrade overwrites.

They go into the ClusterConfiguration of the cluster, so kubeadm applies them to
a node that joins later and to every node an upgrade passes through, rather than
to the nodes that happened to exist when they were set.

  k8s_apiserver_extra_args:
    - name: audit-policy-file
      value: /etc/kubernetes/audit/policy.yaml
    - name: audit-log-path
      value: /var/log/kubernetes/audit/audit.log
  k8s_apiserver_extra_volumes:
    - name: audit
      hostPath: /etc/kubernetes/audit
      mountPath: /etc/kubernetes/audit
      readOnly: true
    - name: audit-log
      hostPath: /var/log/kubernetes/audit
      mountPath: /var/log/kubernetes/audit
      pathType: DirectoryOrCreate

The name of an argument carries no dashes. A flag that can be given more than
once is listed more than once.

A volume is a host path of the control plane node, and nothing here puts a file
there. An argument naming a file that is not on the node is an apiserver that
does not start, and it is a control plane node that does not come back, so put
the file on every control plane node first and change these after`,
	},
	{
		Key:      "k8s_apiserver_extra_volumes",
		File:     FileK8s,
		Group:    "control_plane",
		Type:     TypeList,
		Default:  "[]",
		Short:    "kube-apiserver 추가 host path 볼륨",
		VarClass: "control_plane",
		Long: `Flags and host paths added to the kube-apiserver of every control plane node.
Empty leaves it exactly as kubeadm builds it.

What this is for is the apiserver options a cluster is required to run with and
the installer has no opinion about: an audit policy, an oidc issuer, a feature
gate, a runtime-config. Without it the only way to set one is to edit the static
pod manifest by hand on each node, which the next kubeadm upgrade overwrites.

They go into the ClusterConfiguration of the cluster, so kubeadm applies them to
a node that joins later and to every node an upgrade passes through, rather than
to the nodes that happened to exist when they were set.

  k8s_apiserver_extra_args:
    - name: audit-policy-file
      value: /etc/kubernetes/audit/policy.yaml
    - name: audit-log-path
      value: /var/log/kubernetes/audit/audit.log
  k8s_apiserver_extra_volumes:
    - name: audit
      hostPath: /etc/kubernetes/audit
      mountPath: /etc/kubernetes/audit
      readOnly: true
    - name: audit-log
      hostPath: /var/log/kubernetes/audit
      mountPath: /var/log/kubernetes/audit
      pathType: DirectoryOrCreate

The name of an argument carries no dashes. A flag that can be given more than
once is listed more than once.

A volume is a host path of the control plane node, and nothing here puts a file
there. An argument naming a file that is not on the node is an apiserver that
does not start, and it is a control plane node that does not come back, so put
the file on every control plane node first and change these after`,
	},
	{
		Key:      "k8s_controller_manager_extra_args",
		File:     FileK8s,
		Group:    "control_plane",
		Type:     TypeList,
		Default:  "[]",
		Short:    "kube-controller-manager 추가 인자",
		VarClass: "control_plane",
		Long: `The same for the rest of the control plane, which is configured the same way
and was not reachable at all.

What each one is for, in the order below: the controller manager decides how
large a slice of the pod subnet a node is given (--node-cidr-mask-size), which
is how many pods a node can hold and is not a thing to discover after the
cluster is built. The scheduler carries the feature gates a workload may
require. And etcd holds the quota its database is refused writes at
(--quota-backend-bytes), which defaults to 2GiB and is the wall a long running
cluster meets. Defragmenting gives back the space already deleted; this raises
the wall itself.

  k8s_controller_manager_extra_args:
    - name: node-cidr-mask-size
      value: 22
  k8s_etcd_extra_args:
    - name: quota-backend-bytes
      value: 8589934592

etcd takes arguments and no volumes. A volume there would be a path mounted
into the member holding the state of the cluster, and the one path it reads is
its data directory, which is not a value this installer lets a node move.

Everything the apiserver block above says applies here too: the name of an
argument carries no dashes, a file a volume mounts is not put there by this
installer, and a component that will not start is a control plane node that
does not come back`,
	},
	{
		Key:      "k8s_controller_manager_extra_volumes",
		File:     FileK8s,
		Group:    "control_plane",
		Type:     TypeList,
		Default:  "[]",
		Short:    "kube-controller-manager 추가 host path 볼륨",
		VarClass: "control_plane",
		Long: `The same for the rest of the control plane, which is configured the same way
and was not reachable at all.

What each one is for, in the order below: the controller manager decides how
large a slice of the pod subnet a node is given (--node-cidr-mask-size), which
is how many pods a node can hold and is not a thing to discover after the
cluster is built. The scheduler carries the feature gates a workload may
require. And etcd holds the quota its database is refused writes at
(--quota-backend-bytes), which defaults to 2GiB and is the wall a long running
cluster meets. Defragmenting gives back the space already deleted; this raises
the wall itself.

  k8s_controller_manager_extra_args:
    - name: node-cidr-mask-size
      value: 22
  k8s_etcd_extra_args:
    - name: quota-backend-bytes
      value: 8589934592

etcd takes arguments and no volumes. A volume there would be a path mounted
into the member holding the state of the cluster, and the one path it reads is
its data directory, which is not a value this installer lets a node move.

Everything the apiserver block above says applies here too: the name of an
argument carries no dashes, a file a volume mounts is not put there by this
installer, and a component that will not start is a control plane node that
does not come back`,
	},
	{
		Key:      "k8s_scheduler_extra_args",
		File:     FileK8s,
		Group:    "control_plane",
		Type:     TypeList,
		Default:  "[]",
		Short:    "kube-scheduler 추가 인자",
		VarClass: "control_plane",
		Long: `The same for the rest of the control plane, which is configured the same way
and was not reachable at all.

What each one is for, in the order below: the controller manager decides how
large a slice of the pod subnet a node is given (--node-cidr-mask-size), which
is how many pods a node can hold and is not a thing to discover after the
cluster is built. The scheduler carries the feature gates a workload may
require. And etcd holds the quota its database is refused writes at
(--quota-backend-bytes), which defaults to 2GiB and is the wall a long running
cluster meets. Defragmenting gives back the space already deleted; this raises
the wall itself.

  k8s_controller_manager_extra_args:
    - name: node-cidr-mask-size
      value: 22
  k8s_etcd_extra_args:
    - name: quota-backend-bytes
      value: 8589934592

etcd takes arguments and no volumes. A volume there would be a path mounted
into the member holding the state of the cluster, and the one path it reads is
its data directory, which is not a value this installer lets a node move.

Everything the apiserver block above says applies here too: the name of an
argument carries no dashes, a file a volume mounts is not put there by this
installer, and a component that will not start is a control plane node that
does not come back`,
	},
	{
		Key:      "k8s_scheduler_extra_volumes",
		File:     FileK8s,
		Group:    "control_plane",
		Type:     TypeList,
		Default:  "[]",
		Short:    "kube-scheduler 추가 host path 볼륨",
		VarClass: "control_plane",
		Long: `The same for the rest of the control plane, which is configured the same way
and was not reachable at all.

What each one is for, in the order below: the controller manager decides how
large a slice of the pod subnet a node is given (--node-cidr-mask-size), which
is how many pods a node can hold and is not a thing to discover after the
cluster is built. The scheduler carries the feature gates a workload may
require. And etcd holds the quota its database is refused writes at
(--quota-backend-bytes), which defaults to 2GiB and is the wall a long running
cluster meets. Defragmenting gives back the space already deleted; this raises
the wall itself.

  k8s_controller_manager_extra_args:
    - name: node-cidr-mask-size
      value: 22
  k8s_etcd_extra_args:
    - name: quota-backend-bytes
      value: 8589934592

etcd takes arguments and no volumes. A volume there would be a path mounted
into the member holding the state of the cluster, and the one path it reads is
its data directory, which is not a value this installer lets a node move.

Everything the apiserver block above says applies here too: the name of an
argument carries no dashes, a file a volume mounts is not put there by this
installer, and a component that will not start is a control plane node that
does not come back`,
	},
	{
		Key:      "k8s_etcd_extra_args",
		File:     FileK8s,
		Group:    "control_plane",
		Type:     TypeList,
		Default:  "[]",
		Short:    "etcd 추가 인자",
		VarClass: "control_plane",
		Long: `The same for the rest of the control plane, which is configured the same way
and was not reachable at all.

What each one is for, in the order below: the controller manager decides how
large a slice of the pod subnet a node is given (--node-cidr-mask-size), which
is how many pods a node can hold and is not a thing to discover after the
cluster is built. The scheduler carries the feature gates a workload may
require. And etcd holds the quota its database is refused writes at
(--quota-backend-bytes), which defaults to 2GiB and is the wall a long running
cluster meets. Defragmenting gives back the space already deleted; this raises
the wall itself.

  k8s_controller_manager_extra_args:
    - name: node-cidr-mask-size
      value: 22
  k8s_etcd_extra_args:
    - name: quota-backend-bytes
      value: 8589934592

etcd takes arguments and no volumes. A volume there would be a path mounted
into the member holding the state of the cluster, and the one path it reads is
its data directory, which is not a value this installer lets a node move.

Everything the apiserver block above says applies here too: the name of an
argument carries no dashes, a file a volume mounts is not put there by this
installer, and a component that will not start is a control plane node that
does not come back`,
	},
	{
		Key:      "k8s_cp_extra_files",
		File:     FileK8s,
		Group:    "control_plane",
		Type:     TypeList,
		Default:  "[]",
		Short:    "컨트롤 플레인 노드에 둘 파일 (비밀 금지)",
		VarClass: "cp_extra_files",
		Long: `Files this installer puts on every control plane node, for the volumes above to
carry into the component that declared them. Empty puts nothing anywhere.

A volume declares a path of the node and an argument names a file inside it,
and until now nothing put the file there. That leaves the two halves of one
decision in different places: the argument is cluster wide, so a control plane
node joining later is told to read a file nobody has given it, and a component
that can not read what it was pointed at does not start.

  k8s_cp_extra_files:
    - src: audit/policy.yaml
      dest: /etc/kubernetes/audit/policy.yaml
      mode: "0644"

src is a path under ansible/files of this installer, and that is where the file
itself goes. Relative on purpose: what sits in that directory is carried to
every ki cp node with the rest of the ansible directory and kept across a
release upgrade, while a file somewhere else on the control node is known to
nothing and survives neither.

dest is an absolute path on the node. Owned by root, with the mode given here
or 0644. The directory it sits in is created, and is left behind when the file
is removed.

Taking an entry out removes the file it placed the next time a playbook runs.
Only paths this installer placed are ever removed - what was put on a node by
other means is left alone.

*** not for secrets ***

What goes in ansible/files is copied to every ki cp node in the clear, so that
any of them can take over as the control node. That is the right home for an
audit policy and the wrong one for a key`,
	},
	{
		Key:      "k8s_certificate_validity_period",
		File:     FileK8s,
		Group:    "kubernetes",
		Type:     TypeDuration,
		Default:  "26280h",
		Required: Always,
		Short:    "인증서 유효기간 (예: 26280h = 3년)",
		VarClass: "cluster_config",
		Screen:   "4.K8s",
		Long:     "26280h = 3 year",
	},
	{
		Key:      "ki_preflight_disk_free_min",
		File:     FileK8s,
		Group:    "preflight",
		Type:     TypeSize,
		Default:  "10Gi",
		Short:    "작업 전 필요한 디스크 여유 (null=검사 안 함)",
		VarClass: "no_action",
		Long: `What a node has to have before a playbook starts working on it.

The disk is checked on every filesystem this installer and kubernetes put
things on, and both thresholds have to be met. The absolute one is what an
upgrade actually needs, since it pushes a whole release of images into the
registry of every ki cp node; the percentage is for small disks, where a
filesystem can be over the absolute and still be close enough to the eviction
thresholds of kubelet for pods to start moving.

The clock is checked because certificates are issued against it and etcd
measures its peers by it, so a node that is wrong about the time fails in ways
that name neither. Nothing here sets the clock - the time daemon does - and
what this asks is whether it worked.

A playbook that is taking a cluster apart or rescuing a node that has already
failed only reports these. The operator came to fix or remove, and a full disk
is a reason they came rather than a reason to turn them away.

null is not asking at all. It is here for the site the question does not fit -
an air-gapped one with no time source to synchronise to answers the clock at no
threshold, and there is nothing it could set instead. Setting the clock one to
null drops the synchronised flag with it, since that is the same question`,
	},
	{
		Key:      "ki_preflight_disk_free_percent_min",
		File:     FileK8s,
		Group:    "preflight",
		Type:     TypeInt,
		Default:  "5",
		Short:    "작업 전 필요한 디스크 여유 % (null=검사 안 함)",
		VarClass: "no_action",
		Long: `What a node has to have before a playbook starts working on it.

The disk is checked on every filesystem this installer and kubernetes put
things on, and both thresholds have to be met. The absolute one is what an
upgrade actually needs, since it pushes a whole release of images into the
registry of every ki cp node; the percentage is for small disks, where a
filesystem can be over the absolute and still be close enough to the eviction
thresholds of kubelet for pods to start moving.

The clock is checked because certificates are issued against it and etcd
measures its peers by it, so a node that is wrong about the time fails in ways
that name neither. Nothing here sets the clock - the time daemon does - and
what this asks is whether it worked.

A playbook that is taking a cluster apart or rescuing a node that has already
failed only reports these. The operator came to fix or remove, and a full disk
is a reason they came rather than a reason to turn them away.

null is not asking at all. It is here for the site the question does not fit -
an air-gapped one with no time source to synchronise to answers the clock at no
threshold, and there is nothing it could set instead. Setting the clock one to
null drops the synchronised flag with it, since that is the same question`,
	},
	{
		Key:      "ki_preflight_clock_offset_max_seconds",
		File:     FileK8s,
		Group:    "preflight",
		Type:     TypeInt,
		Default:  "5",
		Short:    "허용하는 시계 오차(초) (null=검사 안 함)",
		VarClass: "no_action",
		Long: `What a node has to have before a playbook starts working on it.

The disk is checked on every filesystem this installer and kubernetes put
things on, and both thresholds have to be met. The absolute one is what an
upgrade actually needs, since it pushes a whole release of images into the
registry of every ki cp node; the percentage is for small disks, where a
filesystem can be over the absolute and still be close enough to the eviction
thresholds of kubelet for pods to start moving.

The clock is checked because certificates are issued against it and etcd
measures its peers by it, so a node that is wrong about the time fails in ways
that name neither. Nothing here sets the clock - the time daemon does - and
what this asks is whether it worked.

A playbook that is taking a cluster apart or rescuing a node that has already
failed only reports these. The operator came to fix or remove, and a full disk
is a reason they came rather than a reason to turn them away.

null is not asking at all. It is here for the site the question does not fit -
an air-gapped one with no time source to synchronise to answers the clock at no
threshold, and there is nothing it could set instead. Setting the clock one to
null drops the synchronised flag with it, since that is the same question`,
	},
	{
		Key:      "ki_etcd_backup_retention_count",
		File:     FileK8s,
		Group:    "preflight",
		Type:     TypeInt,
		Default:  "5",
		Short:    "k8s cp 노드마다 남길 etcd 스냅숏 수",
		VarClass: "no_action",
		Long: `How many etcd snapshots each k8s cp node keeps. The oldest over this are
removed as each new one is taken, because nothing else removes them and they
sit on the root filesystem of a control plane node`,
	},
	{
		Key:      "kubelet_system_reserved_cpu",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeString,
		Short:    "kubelet system reserved cpu (null=노드 용량으로 계산)",
		VarClass: "kubelet",
		Long: `Kubelet resource reservations
Leave a value null to calculate it from the actual cpu, memory and disk capacity of the node.
See scripts/preflight/create-kubelet-reservations.py for the calculation
To give a single node different values, override only that variable in inventory.yml`,
	},
	{
		Key:      "kubelet_system_reserved_memory",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeString,
		Short:    "kubelet system reserved memory (null=노드 용량으로 계산)",
		VarClass: "kubelet",
		Long: `Kubelet resource reservations
Leave a value null to calculate it from the actual cpu, memory and disk capacity of the node.
See scripts/preflight/create-kubelet-reservations.py for the calculation
To give a single node different values, override only that variable in inventory.yml`,
	},
	{
		Key:      "kubelet_system_reserved_ephemeral_storage",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeString,
		Short:    "kubelet system reserved ephemeral storage (null=노드 용량으로 계산)",
		VarClass: "kubelet",
		Long: `Kubelet resource reservations
Leave a value null to calculate it from the actual cpu, memory and disk capacity of the node.
See scripts/preflight/create-kubelet-reservations.py for the calculation
To give a single node different values, override only that variable in inventory.yml`,
	},
	{
		Key:      "kubelet_system_reserved_pid",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeString,
		Short:    "kubelet system reserved pid (null=노드 용량으로 계산)",
		VarClass: "kubelet",
		Long: `Kubelet resource reservations
Leave a value null to calculate it from the actual cpu, memory and disk capacity of the node.
See scripts/preflight/create-kubelet-reservations.py for the calculation
To give a single node different values, override only that variable in inventory.yml`,
	},
	{
		Key:      "kubelet_kube_reserved_cpu",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeString,
		Short:    "kubelet kube reserved cpu (null=노드 용량으로 계산)",
		VarClass: "kubelet",
		Long: `Kubelet resource reservations
Leave a value null to calculate it from the actual cpu, memory and disk capacity of the node.
See scripts/preflight/create-kubelet-reservations.py for the calculation
To give a single node different values, override only that variable in inventory.yml`,
	},
	{
		Key:      "kubelet_kube_reserved_memory",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeString,
		Short:    "kubelet kube reserved memory (null=노드 용량으로 계산)",
		VarClass: "kubelet",
		Long: `Kubelet resource reservations
Leave a value null to calculate it from the actual cpu, memory and disk capacity of the node.
See scripts/preflight/create-kubelet-reservations.py for the calculation
To give a single node different values, override only that variable in inventory.yml`,
	},
	{
		Key:      "kubelet_kube_reserved_ephemeral_storage",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeString,
		Short:    "kubelet kube reserved ephemeral storage (null=노드 용량으로 계산)",
		VarClass: "kubelet",
		Long: `Kubelet resource reservations
Leave a value null to calculate it from the actual cpu, memory and disk capacity of the node.
See scripts/preflight/create-kubelet-reservations.py for the calculation
To give a single node different values, override only that variable in inventory.yml`,
	},
	{
		Key:      "kubelet_kube_reserved_pid",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeString,
		Short:    "kubelet kube reserved pid (null=노드 용량으로 계산)",
		VarClass: "kubelet",
		Long: `Kubelet resource reservations
Leave a value null to calculate it from the actual cpu, memory and disk capacity of the node.
See scripts/preflight/create-kubelet-reservations.py for the calculation
To give a single node different values, override only that variable in inventory.yml`,
	},
	{
		Key:      "kubelet_eviction_hard_memory_available",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeString,
		Short:    "kubelet eviction hard memory available (null=노드 용량으로 계산)",
		VarClass: "kubelet",
		Long: `Kubelet resource reservations
Leave a value null to calculate it from the actual cpu, memory and disk capacity of the node.
See scripts/preflight/create-kubelet-reservations.py for the calculation
To give a single node different values, override only that variable in inventory.yml`,
	},
	{
		Key:      "kubelet_eviction_hard_nodefs_available",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeString,
		Short:    "kubelet eviction hard nodefs available (null=노드 용량으로 계산)",
		VarClass: "kubelet",
		Long: `Kubelet resource reservations
Leave a value null to calculate it from the actual cpu, memory and disk capacity of the node.
See scripts/preflight/create-kubelet-reservations.py for the calculation
To give a single node different values, override only that variable in inventory.yml`,
	},
	{
		Key:      "kubelet_eviction_hard_imagefs_available",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeString,
		Short:    "kubelet eviction hard imagefs available (null=노드 용량으로 계산)",
		VarClass: "kubelet",
		Long: `Kubelet resource reservations
Leave a value null to calculate it from the actual cpu, memory and disk capacity of the node.
See scripts/preflight/create-kubelet-reservations.py for the calculation
To give a single node different values, override only that variable in inventory.yml`,
	},
	{
		Key:      "kubelet_eviction_hard_nodefs_inodes_free",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeString,
		Default:  "5%",
		Short:    "kubelet eviction hard nodefs inodes free (null=노드 용량으로 계산)",
		VarClass: "kubelet",
		Long: `Kubelet resource reservations
Leave a value null to calculate it from the actual cpu, memory and disk capacity of the node.
See scripts/preflight/create-kubelet-reservations.py for the calculation
To give a single node different values, override only that variable in inventory.yml`,
	},
	{
		Key:      "kubelet_auto_memory_available_min",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeSize,
		Default:  "500Mi",
		Short:    "계산한 예약값의 하한 (memory available)",
		VarClass: "kubelet",
		Long: `Bounds applied to the calculated values.
A lower bound is ignored when it would take an excessive share of a small node`,
	},
	{
		Key:      "kubelet_auto_memory_available_max",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeSize,
		Default:  "4Gi",
		Short:    "계산한 예약값의 상한 (memory available)",
		VarClass: "kubelet",
		Long: `Bounds applied to the calculated values.
A lower bound is ignored when it would take an excessive share of a small node`,
	},
	{
		Key:      "kubelet_auto_nodefs_available_min",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeSize,
		Default:  "15Gi",
		Short:    "계산한 예약값의 하한 (nodefs available)",
		VarClass: "kubelet",
		Long: `Bounds applied to the calculated values.
A lower bound is ignored when it would take an excessive share of a small node`,
	},
	{
		Key:      "kubelet_auto_nodefs_available_max",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeSize,
		Default:  "50Gi",
		Short:    "계산한 예약값의 상한 (nodefs available)",
		VarClass: "kubelet",
		Long: `Bounds applied to the calculated values.
A lower bound is ignored when it would take an excessive share of a small node`,
	},
	{
		Key:      "kubelet_auto_ephemeral_storage_min",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeSize,
		Default:  "5Gi",
		Short:    "계산한 예약값의 하한 (ephemeral storage)",
		VarClass: "kubelet",
		Long: `Bounds applied to the calculated values.
A lower bound is ignored when it would take an excessive share of a small node`,
	},
	{
		Key:      "kubelet_auto_ephemeral_storage_max",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeSize,
		Default:  "30Gi",
		Short:    "계산한 예약값의 상한 (ephemeral storage)",
		VarClass: "kubelet",
		Long: `Bounds applied to the calculated values.
A lower bound is ignored when it would take an excessive share of a small node`,
	},
	{
		Key:      "kubelet_cpu_manager_policy",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeEnum,
		Default:  "none",
		Enum:     []string{"none", "static"},
		Short:    "CPU 할당 정책 (static=전용 코어)",
		VarClass: "kubelet_policy",
		Long: `How kubelet hands out the cpus of a node. Set per node in inventory.yml:
whether a node is pinned follows from what it holds, and a node running gpu
virtual machines wants it while the one running the control plane does not.

none, the default, means what it has always meant - a container is given cpu
time and the kernel moves its threads between cores. A workload that needs a
core to itself, a virtual machine with pinned vcpus or a latency sensitive
server, can not be given one that way, and neither can it be lined up with the
numa node its gpu sits on.

static gives whole cpus to containers that ask for whole cpus in a Guaranteed
pod, and is what kubevirt requires for dedicatedCpuPlacement. It costs density:
a cpu given out is a cpu nothing else runs on, idle or not.

Changing it is not a value being rewritten. kubelet checkpoints the policy it
last ran and refuses to start when the new configuration disagrees, saying to
drain the node and delete the file, so update-cluster.yml drains the node,
takes the checkpoint away and brings kubelet back, one node at a time`,
	},
	{
		Key:      "kubelet_cpu_manager_full_pcpus_only",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeBool,
		Default:  "false",
		Short:    "전용 할당을 물리 코어 단위로 (static 일 때)",
		VarClass: "kubelet_policy",
		Long: `Whether an exclusive allocation is a whole physical core rather than one
hardware thread. Read only while the policy above is static.

Left false, a node hands out logical cpus, so the two threads of one core can
go to two different containers - the model a public cloud sells as a vcpu.
Turned on, the threads of a core go together, which halves how many exclusive
cpus a node has and takes the neighbour off the other thread of every core.

A pod asking for an odd number of cpus is then refused rather than given a
thread too many, so turning this on can leave Pending what used to run`,
	},
	{
		Key:      "kubelet_topology_manager_policy",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeEnum,
		Default:  "none",
		Enum:     []string{"none", "best-effort", "restricted", "single-numa-node"},
		Short:    "NUMA 정렬 정책",
		VarClass: "kubelet_policy",
		Long: `Whether kubelet lines the cpus, the memory and the devices of a workload up on
one numa node, and how strictly.

What this is for is a device that sits on a numa node - a gpu that was passed
through, a fabric nic. A container given cpus on the other side of the machine
reaches its own device across the interconnect, and nothing reports that as
anything but a workload that runs slower than it should.

single-numa-node refuses to admit what it can not line up, which is the point:
the alternative is a pod that runs and quietly performs worse. The cpu part of
it needs the policy above to be static to have anything to line up`,
	},
	{
		Key:      "kubelet_topology_manager_scope",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeEnum,
		Default:  "container",
		Enum:     []string{"container", "pod"},
		Short:    "NUMA 정렬 단위",
		VarClass: "kubelet_policy",
		Long: `Whether the alignment above is decided per container or for the pod as a whole.
pod is what a virtual machine wants, since its vcpus and its device are one
workload rather than several`,
	},
	{
		Key:      "kubelet_memory_manager_policy",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeEnum,
		Default:  "none",
		Enum:     []string{"none", "static"},
		Short:    "메모리/hugepage NUMA 할당 정책",
		VarClass: "kubelet_policy",
		Long: `Whether kubelet counts the memory and the hugepages of a node per numa node
and hands them out that way, so that the topology manager above has a memory
hint to line up with the cpus and the devices.

none, the default, leaves memory where it has always been - one pool, placed by
the kernel. static counts the memory and the hugepages of every numa node,
gives a Guaranteed container memory from the numa node its cpus and its device
were lined up with, and refuses what it can not place. It is what kubevirt
needs for a guest to see the numa topology of the host (guestMappingPassthrough),
which in turn is what a virtual machine with a passed through gpu wants: the
guest memory on hugepages of the numa node the gpu sits on.

Only the memory the node keeps for itself is decided here. kubelet asks that the
memory named per numa node add up to exactly what systemReserved, kubeReserved
and the memory.available eviction threshold reserve, so
create-kubelet-reservations.py takes the three it already calculated and
spreads their sum over the numa nodes of the machine. The hugepages themselves
are the kernel's to allocate and the site's to size: declared in
kernel_cmdline_extra_args, in force at a boot, and counted by
check-kernel-args.sh against what the kernel actually took.

Read with the topology manager. With kubelet_topology_manager_policy at none
there is nothing for the memory hint to be lined up with, so the pair is refused.
And it costs what the cpu policy costs: kubelet checkpoints this too, in
/var/lib/kubelet/memory_manager_state, and refuses to start when the
configuration disagrees with the checkpoint - so changing it is the same drain,
the same checkpoint taken away and the same restart, one node at a time. A
memory reservation changed on a node that runs static goes the same way, since
the checkpoint records what was reserved`,
	},
	{
		Key:      "kubelet_reserved_system_cpus",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeString,
		Short:    "시스템용으로 지정할 CPU 목록 (null=개수로만 예약)",
		VarClass: "kubelet_policy",
		Long: `The cpus kept for the system and for kubelet, named rather than counted.

Left null, kubelet reserves as much cpu as the reservations below ask for and
picks which cpus itself. Naming them pins the system to a known set - typically
the cpus of one numa node - and leaves the rest whole for exclusive allocation.

kubelet overwrites the cpu it keeps for the system with the count of the set
named here, so setting this and kubelet_system_reserved_cpu on one node means
one of the two does nothing and the playbooks refuse it. The reservation this
installer calculates is overwritten the same way and is left where it is`,
	},
	{
		Key:      "kubelet_server_tls_bootstrap",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeBool,
		Default:  "false",
		Short:    "kubelet 서빙 인증서를 클러스터에 요청 (승인 준비 뒤에)",
		VarClass: "kubelet",
		Long: `Whether kubelet asks the cluster for the certificate it serves on, rather than
signing one itself.

Left off, every kubelet serves a certificate it made up, so everything that
connects to one either skips verification or fails. That is what
--kubelet-insecure-tls is for in metrics-server, and it is the same hole for
"kubectl top", "kubectl logs" and anything else that reaches a node.

Turned on, kubelet requests one through a certificate signing request at
startup and at every rotation.

*** nothing approves those requests, and the node is down until something does ***

kube-controller-manager signs kubelet-serving requests and deliberately does
not approve them: a node asks for the names and addresses it will be reached
at, and believing that without checking is how one node gets a certificate for
another.

While it waits kubelet does not fall back to one it signed itself. It has
nothing to serve and refuses every handshake on 10250 - "no serving
certificate available for the kubelet" - so kubectl logs, kubectl exec,
kubectl top and metrics-server all stop for that node, and
--kubelet-insecure-tls buys nothing because there is no certificate to skip
verifying. So applying this stops at the first node the cluster does not
answer for, rather than walking the rest of them down behind it: one node is
waiting, the others are untouched, and the run says so.

So this is turned on with the approval already arranged, not ahead of it -
an approver that checks the request against what the cluster knows about the
node, or a person standing by with "kubectl certificate approve". The requests
come back at every rotation, so approving by hand is a standing chore rather
than a step. check-kubelet-serving-cert.yml reports the nodes that are not
serving an issued certificate.

For the apiserver to gain anything from it, it has to be told which ca to
trust: k8s_apiserver_extra_args with kubelet-certificate-authority pointing at
/etc/kubernetes/pki/ca.crt. Without that the apiserver goes on not verifying,
and what this buys is only what other clients verify, which
check-kubelet-serving-cert.yml says when the argument is not there.

Cluster wide on purpose, in this file rather than per node, and the preflight
refuses it on a node rather than leaving that to be read here. A cluster where
some kubelets are verifiable and others are not is one where every client has
to know which is which, and no client is written that way`,
	},
	{
		Key:      "kubelet_max_pods",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeInt,
		Short:    "노드당 최대 파드 수 (null=110)",
		VarClass: "kubelet",
		Long: `The most pods kubelet accepts on a node. Left null, kubernetes decides (110).
A node running many small pods reaches it; a node whose pods are virtual
machines with large cpu and memory never gets near it`,
	},
	{
		Key:      "kubelet_ki_cp_extra_cpu",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeString,
		Default:  "500m",
		Short:    "ki cp 노드에 더 예약할 CPU",
		VarClass: "kubelet",
		Long: `Added to systemReserved on nodes of the ki_cp_node group.
Those nodes run keepalived, dns server, ntp server, k8s cp lb and k8s registry
with docker, outside of kubernetes, so kubelet does not account for them`,
	},
	{
		Key:      "kubelet_ki_cp_extra_memory",
		File:     FileK8s,
		Group:    "kubelet",
		Type:     TypeSize,
		Default:  "1Gi",
		Short:    "ki cp 노드에 더 예약할 메모리",
		VarClass: "kubelet",
		Long: `Added to systemReserved on nodes of the ki_cp_node group.
Those nodes run keepalived, dns server, ntp server, k8s cp lb and k8s registry
with docker, outside of kubernetes, so kubelet does not account for them`,
	},
	{
		Key:       "aipub_load_balancers",
		File:      FileAipub,
		Group:     "ingress",
		Type:      TypeList,
		Default:   `[{name: "lb1", vip: "192.168.0.100", nodes: ["node1", "node2"]}]`,
		Required:  Always,
		Short:     "L2 로 알리는 VIP (nodes 는 인벤토리 이름)",
		Preflight: "nodes 의 이름이 inventory.yml 에 있어야 함",
		Screen:    "6.AIPub LB",
		Long:      "Load balancers announced over L2. nodes are inventory names",
	},
	{
		Key:     "aipub_default_ingress_class",
		File:    FileAipub,
		Group:   "ingress",
		Type:    TypeMap,
		Default: `{load_balancer: "lb1", host_network_port: 443}`,
		Short:   "기본 인그레스 클래스가 쓸 LB / 호스트 포트",
		Screen:  "6.AIPub LB",
		Long: `The default ingress class of cilium shares the VIP of this load balancer.
Set load_balancer to null to listen on host_network_port of every node instead`,
	},
	{
		Key:     "aipub_cilium_envoy_idle_timeout_seconds",
		File:    FileAipub,
		Group:   "ingress",
		Type:    TypeInt,
		Default: "300",
		Short:   "cilium envoy 유휴 타임아웃(초)",
	},
	{
		Key:     "aipub_cilium_hubble_relay_enabled",
		File:    FileAipub,
		Group:   "ingress",
		Type:    TypeBool,
		Default: "false",
		Short:   "hubble relay 설치",
	},
	{
		Key:     "aipub_cilium_hubble_ui_enabled",
		File:    FileAipub,
		Group:   "ingress",
		Type:    TypeBool,
		Default: "false",
		Short:   "hubble ui 설치",
	},
	{
		Key:       "aipub_cert_mode",
		File:      FileAipub,
		Group:     "cert",
		Type:      TypeEnum,
		Default:   "self_signed",
		Enum:      []string{"self_signed", "ca_provided", "tls_provided"},
		Short:     "인증서 방식",
		Preflight: "ca_provided/tls_provided 면 /etc/k8s-installer/certs 에 파일을 먼저 둠",
		Screen:    "7.인증서",
		Long: `self_signed: a CA is made and *.<aipub_ingress_zone> is issued by it
ca_provided: place ca.crt and ca.key in /etc/k8s-installer/certs of the first
             k8s control plane node first; the certificate is issued by it
tls_provided: place ca.crt there and tls.crt, tls.key in
             /etc/k8s-installer/certs/*.<aipub_ingress_zone>/; nothing is issued`,
	},
	{
		Key:   "aipub_preferred_runner_node",
		File:  FileAipub,
		Group: "cert",
		Type:  TypeString,
		Short: "aipub 플레이북을 돌릴 노드 (비우면 첫 k8s cp 노드)",
		Long: `The node the files of ca_provided and tls_provided are placed on, and that the
aipub playbooks run on. Unset, it is the first reachable k8s control plane node
by name; the playbooks print which one as "aipub_runner_node"`,
	},
	{
		Key:       "aipub_ingress_zone",
		File:      FileAipub,
		Group:     "ingress",
		Type:      TypeFQDN,
		Default:   "example.com",
		Required:  Always,
		Short:     "AIPub 도메인 (예: aipub.example.com)",
		Preflight: "`*.<이 도메인>` DNS 가 인그레스 VIP 를 가리켜야 외부에서 접속됨",
		Screen:    "5.AIPub",
		Long:      "Harbor is served at https://<aipub_harbor_ingress_subdomain>.<aipub_ingress_zone>",
	},
	{
		Key:     "aipub_harbor_ingress_subdomain",
		File:    FileAipub,
		Group:   "harbor",
		Type:    TypeSubdomain,
		Default: "aipub-harbor",
		Short:   "Harbor 서브도메인 (https://<여기>.<도메인>)",
		Screen:  "5.AIPub",
		Long:    "Harbor is served at https://<aipub_harbor_ingress_subdomain>.<aipub_ingress_zone>",
	},
	{
		Key:     "aipub_ha_mode",
		File:    FileAipub,
		Group:   "harbor",
		Type:    TypeBool,
		Default: "true",
		Short:   "true=스토리지클래스(RWX) / false=노드 로컬 디렉터리",
		Screen:  "5.AIPub",
		Long:    "HA: the volumes are claims on a storage class the site provides, ReadWriteMany",
	},
	{
		Key:       "aipub_ha_mode_storage_class",
		File:      FileAipub,
		Group:     "harbor",
		Type:      TypeString,
		Default:   "nfs-client",
		Required:  "aipub_ha_mode=true",
		Short:     "HA 볼륨의 StorageClass (ReadWriteMany)",
		Preflight: "`kubectl get storageclass <값>` 이 있어야 함",
		Screen:    "5.AIPub",
		Long:      "HA: the volumes are claims on a storage class the site provides, ReadWriteMany",
	},
	{
		Key:     "aipub_harbor_replica_count",
		File:    FileAipub,
		Group:   "harbor",
		Type:    TypeInt,
		Default: "3",
		Short:   "Harbor 레플리카 수",
		Screen:  "5.AIPub",
		Long:    "HA: the volumes are claims on a storage class the site provides, ReadWriteMany",
	},
	{
		Key:      "aipub_harbor_local_pv_node",
		File:     FileAipub,
		Group:    "harbor",
		Type:     TypeString,
		Required: "aipub_ha_mode=false",
		Short:    "HA 가 아닐 때 Harbor 데이터를 둘 노드 (인벤토리 이름)",
		Screen:   "5.AIPub",
		Long:     "Without HA: the volumes are directories of one labelled node, by inventory name",
	},
	{
		Key:   "aipub_app_storage_class_name",
		File:  FileAipub,
		Group: "app",
		Type:  TypeString,
		Short: "HA 없이 프론트엔드에 보일 StorageClass (기본 aipub_ha_mode_storage_class)",
		Long: `Set it to offer a storage class in the frontend without HA; left unset, only
NFS is offered`,
	},
	{
		Key:     "aipub_harbor_registry_storage_size",
		File:    FileAipub,
		Group:   "harbor",
		Type:    TypeSize,
		Default: "512Gi",
		Short:   "Harbor registry 볼륨 크기",
		Screen:  "5.AIPub",
	},
	{
		Key:     "aipub_harbor_postgresql_storage_size",
		File:    FileAipub,
		Group:   "harbor",
		Type:    TypeSize,
		Default: "128Gi",
		Short:   "Harbor postgresql 볼륨 크기",
		Screen:  "5.AIPub",
	},
	{
		Key:     "aipub_harbor_redis_storage_size",
		File:    FileAipub,
		Group:   "harbor",
		Type:    TypeSize,
		Default: "32Gi",
		Short:   "Harbor redis 볼륨 크기",
		Screen:  "5.AIPub",
	},
	{
		Key:     "aipub_harbor_trivy_storage_size",
		File:    FileAipub,
		Group:   "harbor",
		Type:    TypeSize,
		Default: "5Gi",
		Short:   "Harbor trivy 볼륨 크기",
		Screen:  "5.AIPub",
	},
	{
		Key:   "aipub_harbor_admin_password",
		File:  FileAipub,
		Group: "harbor",
		Type:  TypeString,
		Short: "Harbor 관리자 비밀번호 (여기 말고 vault 에)",
		Long: `Left unset, a password is generated into the secret aipub-harbor-admin of the
aipub namespace when harbor is first installed. Set it only to choose the
password of a new harbor: harbor takes it only when its database is first set up.
This file is copied in plain text to every ki cp node, so a password set here is
readable there; keep it in the vault file of the site`,
	},
	{
		Key:     "aipub_app_host_prefix",
		File:    FileAipub,
		Group:   "app",
		Type:    TypeSubdomain,
		Default: "aipub",
		Short:   "AIPub 서브도메인 (https://<여기>.<도메인>)",
		Long:    "aipub is served at https://<aipub_app_host_prefix>.<aipub_ingress_zone>",
	},
	{
		Key:     "aipub_app_edition",
		File:    FileAipub,
		Group:   "app",
		Type:    TypeEnum,
		Default: "enterprise",
		Enum:    []string{"enterprise", "lite"},
		Short:   "에디션",
		Long: `enterprise or lite: lite leaves aipub-nav out and gives the backend and the
frontend the features of lite only`,
	},
	{
		Key:   "aipub_app_file_storage_storage_class_name",
		File:  FileAipub,
		Group: "app",
		Type:  TypeString,
		Short: "백엔드 공유 볼륨 StorageClass (기본 aipub_app_storage_class_name)",
		Long: `The shared data volume of the backend. dynamic takes a claim on a storage
class, by default aipub_ha_mode_storage_class: set one without HA`,
	},
	{
		Key:     "aipub_app_file_storage_size",
		File:    FileAipub,
		Group:   "app",
		Type:    TypeSize,
		Default: "20Gi",
		Short:   "백엔드 공유 볼륨 크기",
		Long: `The shared data volume of the backend. dynamic takes a claim on a storage
class, by default aipub_ha_mode_storage_class: set one without HA`,
	},
	{
		Key:     "aipub_app_file_storage_type",
		File:    FileAipub,
		Group:   "app",
		Type:    TypeEnum,
		Default: "dynamic",
		Enum:    []string{"dynamic", "static"},
		Short:   "백엔드 공유 볼륨 방식",
		Long: `Or a directory of one node, by its kubernetes node name (kubectl get nodes); setup-aipub.yml makes the
directory on that node`,
	},
	{
		Key:   "aipub_app_file_storage_static_type",
		File:  FileAipub,
		Group: "app",
		Type:  TypeString,
		Short: "static 일 때의 종류 (local)",
		Long: `Or a directory of one node, by its kubernetes node name (kubectl get nodes); setup-aipub.yml makes the
directory on that node`,
	},
	{
		Key:   "aipub_app_file_storage_local_path",
		File:  FileAipub,
		Group: "app",
		Type:  TypeString,
		Short: "static local 일 때 노드의 디렉터리",
		Long: `Or a directory of one node, by its kubernetes node name (kubectl get nodes); setup-aipub.yml makes the
directory on that node`,
	},
	{
		Key:   "aipub_app_file_storage_local_node_hostname",
		File:  FileAipub,
		Group: "app",
		Type:  TypeString,
		Short: "static local 일 때 노드 (kubectl get nodes 의 이름)",
		Long: `Or a directory of one node, by its kubernetes node name (kubectl get nodes); setup-aipub.yml makes the
directory on that node`,
	},
	{
		Key:     "aipub_app_user_manage_enabled",
		File:    FileAipub,
		Group:   "app",
		Type:    TypeBool,
		Default: "false",
		Short:   "오래 쓰지 않은 사용자를 INACTIVE 로 (되돌릴 수 없음)",
		Long:    "Whether long inactive users are moved to INACTIVE, which cannot be undone",
	},
	{
		Key:     "aipub_app_extra_values",
		File:    FileAipub,
		Group:   "app",
		Type:    TypeList,
		Default: "[]",
		Short:   "사이트 값 뒤에 줄 values 파일 (aipub_runner_node 경로)",
		Long: `Values files given to the installer after the site values, on aipub_runner_node. A variable of an
application goes under env/secretEnv of its chart by its own name (README.md, "Changing a value for one site")`,
	},
	{
		Key:     "aipub_app_deploy_tool",
		File:    FileAipub,
		Group:   "deploy",
		Type:    TypeEnum,
		Default: "installer",
		Enum:    []string{"installer", "helmfile"},
		Short:   "AIPub 배포 도구",
		Long:    "installer (default) or helmfile: the same releases and objects, deployed by helmfile sync",
	},
	{
		Key:   "aipub_app_helmfile_bin",
		File:  FileAipub,
		Group: "deploy",
		Type:  TypeString,
		Short: "다른 helmfile 을 쓸 때만 (기본 패키지의 것)",
		Long:  "The package lays helmfile there; set only to use another one",
	},
	{
		Key:   "aipub_app_backoffice_admin_password",
		File:  FileAipub,
		Group: "app",
		Type:  TypeString,
		Short: "백오피스 관리자 비밀번호 (평문으로 복사됨)",
		Long:  "The back office admin. This file is copied in plain text to every ki cp node",
	},
	{
		Key:     "aipub_monitoring_deploy_tool",
		File:    FileAipub,
		Group:   "deploy",
		Type:    TypeEnum,
		Default: "installer",
		Enum:    []string{"installer", "helmfile"},
		Short:   "모니터링 배포 도구",
		Long: `installer (default) or helmfile: the same releases and objects, deployed by helmfile sync around presync.sh and
postsync.sh. Binaries, dry-run, results and how to go back: helmfile/monitoring/README.md, "Field use"`,
	},
	{
		Key:     "aipub_monitoring_helm_bin",
		File:    FileAipub,
		Group:   "deploy",
		Type:    TypeString,
		Default: "helm",
		Short:   "모니터링이 쓰는 helm",
		Long: `installer (default) or helmfile: the same releases and objects, deployed by helmfile sync around presync.sh and
postsync.sh. Binaries, dry-run, results and how to go back: helmfile/monitoring/README.md, "Field use"`,
	},
	{
		Key:     "aipub_monitoring_helmfile_dry_run",
		File:    FileAipub,
		Group:   "deploy",
		Type:    TypeBool,
		Default: "false",
		Short:   "helmfile 을 dry-run 으로만",
		Long: `installer (default) or helmfile: the same releases and objects, deployed by helmfile sync around presync.sh and
postsync.sh. Binaries, dry-run, results and how to go back: helmfile/monitoring/README.md, "Field use"`,
	},
	{
		Key:   "aipub_monitoring_storage_class_name",
		File:  FileAipub,
		Group: "monitoring",
		Type:  TypeString,
		Short: "모니터링 볼륨 StorageClass (기본 aipub_ha_mode_storage_class)",
		Long: `The volumes of OpenSearch and VictoriaMetrics. With HA, claims on a storage
class, by default aipub_ha_mode_storage_class; empty takes the default one`,
	},
	{
		Key:   "aipub_monitoring_storage_type",
		File:  FileAipub,
		Group: "monitoring",
		Type:  TypeEnum,
		Enum:  []string{"dynamic", "static"},
		Short: "모니터링 볼륨 방식 (기본: HA 면 dynamic, 아니면 static)",
		Long:  "Or directories of the k8s control plane nodes (the default without HA)",
	},
	{
		Key:     "aipub_monitoring_storage_static_type",
		File:    FileAipub,
		Group:   "monitoring",
		Type:    TypeEnum,
		Default: "local",
		Enum:    []string{"local", "nfs"},
		Short:   "static 일 때의 종류",
		Long:    "Or directories of the k8s control plane nodes (the default without HA)",
	},
	{
		Key:   "aipub_monitoring_nfs_server",
		File:  FileAipub,
		Group: "monitoring",
		Type:  TypeString,
		Short: "static nfs 일 때 NFS 서버",
		Long:  "Or directories of an NFS export",
	},
	{
		Key:     "aipub_monitoring_opensearch_storage_size",
		File:    FileAipub,
		Group:   "monitoring",
		Type:    TypeSize,
		Default: "3Ti",
		Short:   "OpenSearch 볼륨 크기 (사이트 규모에 맞게)",
	},
	{
		Key:     "aipub_monitoring_opensearch_log_retention",
		File:    FileAipub,
		Group:   "monitoring",
		Type:    TypeString,
		Default: "180d",
		Short:   "OpenSearch 로그 보존 기간",
	},
	{
		Key:     "aipub_monitoring_metrics_storage_size",
		File:    FileAipub,
		Group:   "monitoring",
		Type:    TypeSize,
		Default: "1Ti",
		Short:   "VictoriaMetrics 볼륨 크기 (사이트 규모에 맞게)",
	},
	{
		Key:     "aipub_monitoring_metrics_retention_period",
		File:    FileAipub,
		Group:   "monitoring",
		Type:    TypeString,
		Default: "180d",
		Short:   "메트릭 보존 기간",
	},
	{
		Key:     "aipub_monitoring_opensearch_disk_watermark_low",
		File:    FileAipub,
		Group:   "monitoring",
		Type:    TypeString,
		Default: "85%",
		Short:   "OpenSearch 디스크 워터마크 low",
		Long: `Disk watermarks of OpenSearch: a percentage of use, or the free space to keep
("500gb") on a large disk shared with other data`,
	},
	{
		Key:     "aipub_monitoring_opensearch_disk_watermark_high",
		File:    FileAipub,
		Group:   "monitoring",
		Type:    TypeString,
		Default: "90%",
		Short:   "OpenSearch 디스크 워터마크 high",
		Long: `Disk watermarks of OpenSearch: a percentage of use, or the free space to keep
("500gb") on a large disk shared with other data`,
	},
	{
		Key:     "aipub_monitoring_opensearch_disk_watermark_flood_stage",
		File:    FileAipub,
		Group:   "monitoring",
		Type:    TypeString,
		Default: "95%",
		Short:   "OpenSearch 디스크 워터마크 flood_stage",
		Long: `Disk watermarks of OpenSearch: a percentage of use, or the free space to keep
("500gb") on a large disk shared with other data`,
	},
}
