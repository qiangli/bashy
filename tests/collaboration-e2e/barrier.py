"""Optional file barrier shared with the umbrella runner."""
import json
import os
from pathlib import Path
import time


def hold(name, origin, path):
    control = os.environ.get("COLLAB_E2E_CONTROL_DIR")
    if not control:
        return
    directory = Path(control)
    (directory / (name + ".ready")).write_text(json.dumps({"origin": origin, "path": path}))
    deadline = time.monotonic() + 180
    while time.monotonic() < deadline:
        release = directory / (name + ".release")
        if release.exists() and release.read_text().strip() == "release":
            return
        time.sleep(0.1)
    raise TimeoutError("umbrella release timeout: " + name)
