package state

type NodeConfig struct {
	Name        string
	AnsibleHost string
	AnsiblePort string // empty = use constant-vars default (22)
	AnsibleUser string // empty = use constant-vars default (root)
}

type LBConfig struct {
	Name  string
	VIP   string
	Nodes []string
}

type IngressConfig struct {
	LoadBalancer string
	Port         int
}

type ARecord struct {
	Name string
	IP   string
}
