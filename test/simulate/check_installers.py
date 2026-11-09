#!/usr/bin/env python3
"""Checks the files k8s-installer-tui rendered against what the installers accept,
with their own code where it can run without a cluster.

  check_installers.py <rendered ansible dir> <k8s-installer repo>

k8s-installer (scripts/preflight/validate-hostvars.py):
  - VarsModel: the types and patterns of every variable of vars.yml
  - validate_var_classes: no variable in vars.yml or inventory.yml that no class
    covers, and none that was removed (nvidia_gpu, ...)
aipub-installer (tasks/common/load-site-vars.yml):
  - files/aipub/vars.yml holds aipub_* only, and not the catalog
"""
import sys
from pathlib import Path

import yaml

out, k8s_repo = Path(sys.argv[1]), Path(sys.argv[2])
failed = False


def report(ok, what, details=()):
    global failed
    print(("✓ " if ok else "✗ ") + what)
    for d in details:
        print("    " + d)
    failed |= not ok


# validate-hostvars.py calls main() as its last line; load everything above it, as
# a module pydantic can find by name (it resolves the annotations of the models there)
import types

src = (k8s_repo / "scripts/preflight/validate-hostvars.py").read_text()
src = src[: src.rstrip().rfind("\nmain()")]
module = types.ModuleType("validate_hostvars")
sys.modules["validate_hostvars"] = module
exec(compile(src, "validate-hostvars.py", "exec"), module.__dict__)
ns = module.__dict__

vars_yml = yaml.safe_load((out / "group_vars/all/vars.yml").read_text())
try:
    ns["VarsModel"].model_validate(vars_yml)
    report(True, "k8s-installer VarsModel accepts group_vars/all/vars.yml")
except ns["ValidationError"] as e:
    report(False, "k8s-installer VarsModel refuses group_vars/all/vars.yml",
           [f"{'.'.join(map(str, err['loc']))}: {err['msg']}" for err in e.errors()])

constant = yaml.safe_load((k8s_repo / "ansible/group_vars/all/constant-vars.yml").read_text())
errors = []
ns["validate_var_classes"](errors, {"localhost": {
    "ki_var_classes": constant["ki_var_classes"],
    "ki_removed_vars": constant.get("ki_removed_vars") or {},
    "ki_opt_ansible_path": str(out),
}})
report(not errors, "k8s-installer var classes: every variable of vars.yml and inventory.yml is classified",
       [f"{e.ih}: {'.'.join(map(str, e.location))}: {e.msg}" for e in errors])

aipub = out / "files/aipub/vars.yml"
if aipub.exists():
    site = yaml.safe_load(aipub.read_text()) or {}
    bad = [k for k in site if not k.startswith("aipub_") or k == "aipub_catalog"]
    report(not bad, "aipub-installer: files/aipub/vars.yml holds aipub_* only, without the catalog",
           [f"{k} is refused by load-site-vars.yml" for k in bad])
else:
    print("· no files/aipub/vars.yml (AIPub not installed)")

sys.exit(1 if failed else 0)
