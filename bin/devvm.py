"""Shared helpers for bin/create and bin/destroy.

All external work goes through command line tools: `limactl` and `gh`.
VM metadata lives in a JSON state file under ~/.config/dev-vm.
"""

import json
import re
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
TEMPLATE = REPO / "lima" / "dev-vm.yaml"
KEY_DIR = REPO / "lima" / "tmp"
STATE_DIR = Path.home() / ".config" / "dev-vm"
STATE_FILE = STATE_DIR / "state.json"
STATE_VERSION = 1
NAME_RE = re.compile(r"^[A-Za-z0-9]+(?:[._-][A-Za-z0-9]+)*$")
SCOPE_HINT = "run: gh auth refresh -h github.com -s admin:public_key"


def die(msg):
    print(f"error: {msg}", file=sys.stderr)
    sys.exit(1)


def run(cmd, **kwargs):
    return subprocess.run(cmd, check=True, text=True, capture_output=True, **kwargs)


def now():
    return datetime.now(timezone.utc).isoformat(timespec="seconds")


def check_name(name):
    if not NAME_RE.match(name):
        die(f"invalid VM name {name!r}")


def key_paths(name):
    return KEY_DIR / name, KEY_DIR / f"{name}.pub"


def limactl(*args, capture=True):
    try:
        if capture:
            return run(["limactl", *args]).stdout
        subprocess.run(["limactl", *args], check=True)
        return ""
    except FileNotFoundError:
        die("limactl not found; install Lima")
    except subprocess.CalledProcessError as e:
        die(f"limactl {' '.join(args)} failed: {(e.stderr or '').strip()}")


def vm_exists(name):
    return name in limactl("list", "--quiet").split()


def gh(*args):
    try:
        return run(["gh", *args]).stdout
    except FileNotFoundError:
        die("gh not found; install GitHub CLI")
    except subprocess.CalledProcessError as e:
        err = (e.stderr or "").strip()
        hint = f"\n{SCOPE_HINT}" if "HTTP 403" in err or "HTTP 404" in err else ""
        die(f"gh {' '.join(args)} failed: {err}{hint}")


def gh_json(*args):
    out = gh(*args).strip()
    return json.loads(out) if out else None


def check_scopes():
    for line in gh("api", "-i", "user").splitlines():
        if not line.strip():
            break
        if line.lower().startswith("x-oauth-scopes:"):
            scopes = {s.strip() for s in line.split(":", 1)[1].split(",") if s.strip()}
            if scopes and "admin:public_key" not in scopes:
                die(f"token lacks admin:public_key scope; {SCOPE_HINT}")
            return


def list_keys():
    return gh_json("api", "--paginate", "user/keys") or []


def add_key(title, pub):
    return gh_json("api", "--method", "POST", "user/keys",
                   "-f", f"title={title}", "-f", f"key={pub}")


def delete_key(key_id):
    gh("api", "--method", "DELETE", f"user/keys/{key_id}", "--silent")


def load_state():
    if not STATE_FILE.exists():
        return {"version": STATE_VERSION, "vms": {}}
    try:
        state = json.loads(STATE_FILE.read_text())
    except (OSError, ValueError) as e:
        die(f"cannot read state {STATE_FILE}: {e}")
    state.setdefault("version", STATE_VERSION)
    state.setdefault("vms", {})
    return state


def save_state(state):
    STATE_DIR.mkdir(parents=True, exist_ok=True, mode=0o700)
    tmp = STATE_FILE.with_name(STATE_FILE.name + ".tmp")
    tmp.write_text(json.dumps(state, indent=2, sort_keys=True) + "\n")
    tmp.chmod(0o600)
    tmp.replace(STATE_FILE)


def get_vm(name):
    return load_state()["vms"].get(name, {})


def put_vm(name, **fields):
    state = load_state()
    entry = state["vms"].setdefault(name, {})
    entry.update(fields)
    entry["name"] = name
    entry.setdefault("created_at", now())
    entry["updated_at"] = now()
    save_state(state)
    return entry


def drop_vm(name):
    state = load_state()
    if state["vms"].pop(name, None) is None:
        return False
    save_state(state)
    return True
