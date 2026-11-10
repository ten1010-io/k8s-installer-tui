# templates

Copies of the installers' example files. `writer.go` starts a new site file from
them and adds the keys they have to a site file that lacks them, so they must
match what the installers accept now.

| File | Copy of | Branch |
|------|---------|--------|
| `k8s-vars.yml` | `ansible/group_vars/all/vars.yml` of k8s-installer | `main` |
| `aipub-vars.yml` | `ansible/aipub_vars/examples/vars.yml` of aipub-installer (defaults: `ansible/aipub_vars/defaults/*.yml`) | the branch carrying the IN-70 layout (`feat/IN-70-staged-packager`, then `main` once merged) |
| `authentication-config.yaml` | `scripts/aipub/installer/manifests/authentication-config.yaml` of aipub-installer, the config `k8s-jwks` fills in; the JWKS preset fills in the same two places (issuer URL, CA) | as above |
| `audit-policy.yaml` | `scripts/ki-cp-node/services/python/5_1_x/stages/04-cluster-config/audit-policy.yaml` of aipub-installer | AIP-2611 |

Re-sync them when the installers change (copy the file as it is, then run
`go test ./...`). They use the new `aipub_app_*` names; a site file that still has
the old `aipub_java_*` names loads and is saved with those keys untouched, since
the tool edits the file in place and manages none of them.
