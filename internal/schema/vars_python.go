package schema

// The variables of the python module of aipub-installer (setup-python.yml):
// coaster, usage-manager, workload-controller-manager, the inference and
// workload Gateways, and aipub-network-operator on an InfiniBand site. They are
// the last section of the template, so they follow the monitoring ones.
//
// Only the ones with a Screen are written by the wizard. The rest are here so
// that the site-values view lists them and a site file that sets them is read as
// one the wizard knows: the wizard edits the file in place and leaves them as
// they are.
func init() {
	vars = append(vars, pythonVars...)
}

// PythonScreen is the wizard step the python variables are edited on.
const PythonScreen = "5c.python"

var pythonVars = []Var{
	{
		Key:     "aipub_python_coaster_cluster_name",
		File:    FileAipub,
		Group:   "python",
		Type:    TypeString,
		Default: "cluster1",
		Short:   "Cluster name the coaster license keys were issued for",
		Long:    `Checked against the license keys: the cluster name they were issued for`,
		Preflight: "the coaster license keys of this cluster are issued for this name; coaster refuses a key of" +
			" another name",
		Screen: PythonScreen,
	},
	{
		Key:      "aipub_python_coaster_gpu_nodes",
		File:     FileAipub,
		Group:    "python",
		Type:     TypeList,
		Default:  "[]",
		Optional: true,
		Short:    "GPU nodes, by kubernetes node name (coaster labels them)",
		Long:     `GPU nodes, by kubernetes node name: coaster labels them as GPU nodes`,
		Screen:   PythonScreen,
	},
	{
		Key:   "aipub_python_coaster_license_keys",
		File:  FileAipub,
		Group: "python",
		Type:  TypeMap,
		Short: "coaster license keys by node name (unset: patched by hand after the install)",
	},
	{
		Key:   "aipub_python_workload_gateway_load_balancer",
		File:  FileAipub,
		Group: "python",
		Type:  TypeString,
		Short: "Load balancer of the workload Gateway (unset: the one of the ingress, on 8080/8443)",
		Long: `Where the workload Gateway takes its VIP from: a name of aipub_load_balancers. Unset: the one of the ingress,
shared on 8080/8443`,
		Preflight: "a name of aipub_load_balancers",
	},
	{
		Key:       "aipub_python_cluster_volume_storage_classes",
		File:      FileAipub,
		Group:     "python",
		Type:      TypeList,
		Short:     "Storage classes wcm makes ClusterVolumes on (RWX, Immediate)",
		Long:      `Labeled for ClusterVolumes: wcm makes ClusterVolumes only on these storage classes (RWX, Immediate)`,
		Preflight: "each storage class exists and binds Immediate",
	},
	{
		Key:     "aipub_python_wcm_tcp_visibility_enabled",
		File:    FileAipub,
		Group:   "python",
		Type:    TypeBool,
		Default: "true",
		Short:   "Show the TCP services of a workload to its project",
		Long:    `TCP services shown to the project. On by default`,
	},
	{
		Key:     "aipub_python_wcm_llm_label_propagation_enabled",
		File:    FileAipub,
		Group:   "python",
		Type:    TypeBool,
		Default: "true",
		Short:   "Pass the LLM labels on to the workloads",
		Long:    `LLM labels passed on to the workloads. On by default`,
	},
	{
		Key:   "aipub_python_extra_values",
		File:  FileAipub,
		Group: "python",
		Type:  TypeList,
		Short: "Extra values files of the python chart, on the node the installer runs on",
	},
	{
		Key:      "aipub_python_deploy_tool",
		File:     FileAipub,
		Group:    "deploy",
		Type:     TypeEnum,
		Enum:     []string{"installer", "dag", "helmfile"},
		Default:  "installer",
		Short:    "python deploy tool",
		Optional: true,
		Screen:   PythonScreen,
	},
	{
		Key:     "aipub_python_project_controller_replicas",
		File:    FileAipub,
		Group:   "python",
		Type:    TypeInt,
		Default: "1",
		Short:   "project-controller replicas after the python services (0: scale by hand)",
	},
	{
		Key:      "aipub_python_network_operator_enabled",
		File:     FileAipub,
		Group:    "python",
		Type:     TypeBool,
		Default:  "false",
		Short:    "Install aipub-network-operator (InfiniBand sites), and multus with setup-network",
		Long:     `Installs aipub-network-operator (InfiniBand ports for the pods), and multus with setup-network`,
		Optional: true,
		Screen:   PythonScreen,
	},
	{
		Key:       "aipub_python_network_operator_ib_nodes",
		File:      FileAipub,
		Group:     "python",
		Type:      TypeMap,
		Short:     "InfiniBand nodes and their fabric name (required with the network operator)",
		Preflight: "search-ib-pfs.sh --all on a node prints the value to paste",
	},
	{
		Key:       "aipub_python_network_operator_ib_pfs",
		File:      FileAipub,
		Group:     "python",
		Type:      TypeMap,
		Short:     "InfiniBand ports given to the pods, by node (required with the network operator)",
		Preflight: "search-ib-pfs.sh --all on a node prints the value to paste",
	},
	{
		Key:       "aipub_python_network_operator_default_ipoib_cidr",
		File:      FileAipub,
		Group:     "python",
		Type:      TypeString,
		Default:   "10.250.0.0/16",
		Short:     "IP range of the pods on InfiniBand (empty: RDMA only)",
		Preflight: "does not overlap the pod, service, node or storage ranges of the site",
	},
	{
		Key:     "aipub_python_network_operator_mode",
		File:    FileAipub,
		Group:   "python",
		Type:    TypeEnum,
		Enum:    []string{"pf", "vf"},
		Default: "pf",
		Short:   "Network operator mode (pf, or vf for SR-IOV)",
	},
	{
		Key:     "aipub_python_network_operator_num_vfs_per_pf",
		File:    FileAipub,
		Group:   "python",
		Type:    TypeInt,
		Default: "8",
		Short:   "VFs per PF, as set in the firmware (vf mode)",
	},
	{
		Key:   "aipub_python_network_operator_sm_nodes",
		File:  FileAipub,
		Group: "python",
		Type:  TypeMap,
		Short: "Nodes that run opensm and their priority (vf mode)",
	},
}
