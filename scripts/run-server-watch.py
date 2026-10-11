"""Rebuild and restart the local server when anything under server/ changes."""

import hashlib
import os
from pathlib import Path
import signal
import subprocess
import tempfile
import time


ROOT = Path(__file__).resolve().parent.parent
SOURCE = ROOT / "server"
OUTPUT = ROOT / ".local-server"
BINARY = OUTPUT / "trackharbor-server"


def fingerprint():
    digest = hashlib.sha256()

    def fail(error):
        raise error

    for directory, children, files in os.walk(SOURCE, onerror=fail):
        children.sort()
        files.sort()
        for name in children + files:
            path = Path(directory) / name
            info = path.lstat()
            digest.update(str(path.relative_to(SOURCE)).encode() + b"\0")
            digest.update(f"{info.st_mode}:{info.st_mtime_ns}:{info.st_size}".encode() + b"\0")
            if path.is_symlink():
                digest.update(os.readlink(path).encode())
            elif path.is_file():
                with path.open("rb") as contents:
                    for chunk in iter(lambda: contents.read(65536), b""):
                        digest.update(chunk)
    return digest.digest()


def stop(process):
    if process is None or process.poll() is not None:
        return
    process.terminate()
    try:
        process.wait(timeout=5)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait()


def build():
    fd, path = tempfile.mkstemp(prefix=".trackharbor-server-", dir=OUTPUT)
    os.close(fd)
    try:
        result = subprocess.run(
            ["go", "build", "-o", path, "./server/cmd/server/"], cwd=ROOT
        )
        if result.returncode != 0:
            return False
        os.chmod(path, 0o700)
        os.replace(path, BINARY)
        return True
    finally:
        if os.path.exists(path):
            os.unlink(path)


def interrupt(signum, frame):
    raise KeyboardInterrupt


def main():
    OUTPUT.mkdir(parents=True, exist_ok=True)
    signal.signal(signal.SIGTERM, interrupt)
    process = None
    built_from = None
    print("Watching server/ for changes. Press Ctrl+C to stop.", flush=True)
    try:
        while True:
            try:
                current = fingerprint()
                if current != built_from:
                    print("Building server…", flush=True)
                    if build():
                        stop(process)
                        process = subprocess.Popen([str(BINARY)], cwd=ROOT)
                        built_from = current
                        print("Server restarted.", flush=True)
                    else:
                        print("Build failed; retrying in 1 second.", flush=True)
                elif process is not None and process.poll() is not None:
                    print("Server exited; restarting.", flush=True)
                    process = subprocess.Popen([str(BINARY)], cwd=ROOT)
            except OSError as error:
                print(f"Watcher error: {error}; retrying in 1 second.", flush=True)
            time.sleep(1)
    except KeyboardInterrupt:
        print("Stopping server watcher.", flush=True)
    finally:
        stop(process)


if __name__ == "__main__":
    main()
