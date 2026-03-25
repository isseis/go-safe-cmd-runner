#!/usr/bin/env python3
"""Check that devcontainer.json files are in sync except for the 'name' field."""

import json
import sys
from pathlib import Path

DEVCONTAINER_FILES = [
    Path(".devcontainer/amd64/devcontainer.json"),
    Path(".devcontainer/arm64/devcontainer.json"),
]

IGNORED_KEYS = {"name"}


def load_without_ignored(path: Path) -> dict:
    data = json.loads(path.read_text())
    return {k: v for k, v in data.items() if k not in IGNORED_KEYS}


def main() -> int:
    try:
        contents = {p: load_without_ignored(p) for p in DEVCONTAINER_FILES}
    except FileNotFoundError as e:
        print(f"ERROR: {e}", file=sys.stderr)
        return 1

    files = list(contents.keys())
    reference = json.dumps(contents[files[0]], sort_keys=True, indent=2)

    errors = []
    for path in files[1:]:
        candidate = json.dumps(contents[path], sort_keys=True, indent=2)
        if candidate != reference:
            errors.append(path)

    if errors:
        print("ERROR: devcontainer.json files are out of sync (excluding 'name').")
        print(f"  Reference: {files[0]}")
        for path in errors:
            print(f"  Differs:   {path}")
        print("Fix: apply the same change to all devcontainer.json files.")
        return 1

    return 0


if __name__ == "__main__":
    sys.exit(main())
