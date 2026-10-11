"""Rebuild and restart the local server when server source changes."""

import hashlib
import os
from pathlib import Path
import signal
import subprocess
import tempfile
import time


ROOT = Path(__file__).resolve().parent.parent
SOURCES = [ROOT / name for name in (
    "cmd", "internal", "audio", "download", "spotify", "vk", "youtube",
    "go.mod", "go.sum",
)]
CSS_SOURCES = [ROOT / "internal/web" / name for name in (
    "tailwind.css", "views.go", "assets/app.css", "assets/theme.css",
    "assets/download.js", "assets/player.js", "assets/settings.js",
    "assets/theme.js", "assets/duration.js",
)] + [ROOT / "package.json", ROOT / "package-lock.json"]
CSS_CLI = ROOT / "node_modules/.bin/tailwindcss"
CSS_OUTPUT = ROOT / "internal/web/assets/tailwind.css"
OUTPUT = ROOT / ".local-server"
BINARY = OUTPUT / "trackharbor-server"


def fingerprint(sources=SOURCES):
    digest = hashlib.sha256()

    def fail(error):
        raise error

    for source in sources:
        paths = [source]
        if source.is_dir():
            for directory, children, files in os.walk(source, onerror=fail):
                children.sort()
                files.sort()
                paths.extend(Path(directory) / name for name in children + files)
        for path in paths:
            info = path.lstat()
            digest.update(str(path.relative_to(ROOT)).encode() + b"\0")
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
            ["go", "build", "-o", path, "./cmd/server/"], cwd=ROOT
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
    css_built_from = fingerprint(CSS_SOURCES)
    missing_css_warned = False
    if CSS_CLI.is_file() and (not CSS_OUTPUT.exists() or
            any(path.stat().st_mtime_ns > CSS_OUTPUT.stat().st_mtime_ns
                for path in CSS_SOURCES)):
        css_built_from = None
    print("Watching server source for changes. Press Ctrl+C to stop.", flush=True)
    try:
        while True:
            try:
                css_current = fingerprint(CSS_SOURCES)
                if css_current != css_built_from:
                    if not CSS_CLI.is_file():
                        if not missing_css_warned:
                            print("CSS changed; run npm ci to enable Tailwind builds.", flush=True)
                            missing_css_warned = True
                        time.sleep(1)
                        continue
                    result = subprocess.run(["npm", "run", "css:build"], cwd=ROOT)
                    if result.returncode != 0:
                        print("CSS build failed; keeping the previous server.", flush=True)
                        time.sleep(1)
                        continue
                    css_built_from = css_current
                    missing_css_warned = False
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
