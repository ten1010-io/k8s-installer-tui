package state

type NodeConfig struct {
	Name        string
	AnsibleHost string
	AnsiblePort string // empty = use constant-vars default (22)
	AnsibleUser string // empty = use constant-vars default (root)
}

type ARecord struct {
	Name string
	IP   string
}
