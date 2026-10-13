#!/usr/bin/env python3
"""Record offline verification only after SDK tests and vet actually pass."""
import hashlib
import json
import subprocess
from pathlib import Path

root = Path(__file__).resolve().parents[1]


def run(*args):
    subprocess.run(args, cwd=root, check=True)


run("go", "generate", "./catalog")
run("go", "test", "-race", "./...")
run("go", "vet", "./...")
manifest = json.loads((root / "catalog/manifest.json").read_text())
bindings = json.loads((root / "catalog/bindings.json").read_text())
evidence = {}
for operation in manifest:
    operation_id = operation["operation_id"]
    binding = bindings.get(operation_id)
    if not binding:
        continue
    digest = hashlib.sha256()
    for name in sorted(binding["evidence_files"]):
        digest.update(name.encode())
        digest.update(b"\0")
        digest.update((root / name).read_bytes())
        digest.update(b"\0")
    evidence[operation_id] = {
        "level": "offline",
        "result": "passed",
        "contract_revision": operation["contract_revision"],
        "implementation_revision": operation["implementation_revision"]
        if "implementation_revision" in operation
        else operation["verification"]["implementation_revision"],
        "evidence_revision": digest.hexdigest(),
        "evidence_ref": binding["evidence_ref"],
        "commands": ["go test -race ./...", "go vet ./..."],
    }
(root / "catalog/verification.json").write_text(
    json.dumps(evidence, sort_keys=True, indent=2) + "\n"
)
run("go", "generate", "./catalog")
print(f"Offline verification recorded for {len(evidence)} bindings")
