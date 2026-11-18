#!/usr/bin/env python3
"""Exercise a built CLI without authenticating or changing host network settings."""
import concurrent.futures
import http.server
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import threading
import time

binary = str(Path(sys.argv[1] if len(sys.argv) > 1 else "bin/subscale").resolve())
proxy_address = os.environ.get("SUBSCALE_SMOKE_PROXY_ADDR", "127.0.0.1:18765")
proxy_host, proxy_port = proxy_address.rsplit(":", 1)
assert proxy_host == "127.0.0.1"
proxy_port = int(proxy_port)
# The production proxy address is fixed; do not interfere with an existing daemon.
with socket.socket() as probe:
    try:
        probe.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        probe.bind((proxy_host, proxy_port))
    except OSError:
        raise SystemExit(f"Port {proxy_port} is occupied; use an isolated test build or stop the existing proxy.")

class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        payload = b"subscale direct path works"
        self.send_response(200)
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, *args):
        pass

with tempfile.TemporaryDirectory(prefix="subscale-smoke-", dir="/tmp") as directory:
    env = dict(os.environ, SUBSCALE_HOME=directory, SUBSCALE_NO_BROWSER="1")
    Path(directory, "config.json").write_text(json.dumps({
        "accounts": {"work": {"enabled": False}, "home": {"enabled": False}},
        "routes": [],
    }))

    def run(*args, data=None, check=True):
        result = subprocess.run([binary, *args], env=env, input=data, capture_output=True, timeout=25)
        if check and result.returncode:
            raise AssertionError(f"{args}: {result.stderr.decode()}")
        return result

    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        assert b"subscale" in run("version").stdout
        # Concurrent first commands must converge on one daemon via the file lock.
        with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
            for output in pool.map(lambda _: run("start"), range(3)):
                assert b"LoggedOut" in output.stdout
        assert b"No logged-in accounts" in run("list").stdout
        # Diagnostics are read-only and must not attempt a logged-out account.
        before_diagnostics = Path(directory, "config.json").read_bytes()
        result = run("work", "check", "db.invalid:5432", check=False)
        assert result.returncode != 0
        assert b"LoggedOut" in result.stdout and b"skipped" in result.stdout
        assert b"subscale work login" in result.stdout
        result = run("work", "devices", check=False)
        assert result.returncode != 0 and b"logged out" in result.stderr
        assert run("work", "check", "invalid", check=False).returncode != 0
        assert Path(directory, "config.json").read_bytes() == before_diagnostics
        for reserved in ("status", "list", "update", "upgrade", "rename", "login"):
            result = run(reserved, "login", check=False)
            assert result.returncode != 0 and b"reserved account name" in result.stderr
        assert (proxy_address + "/proxy.pac").encode() in run("proxy", "pac").stdout
        run("work", "add", "https://DB.invalid:8443/path", "--to", "100.64.1.2")
        saved = json.loads(Path(directory, "config.json").read_text())
        assert saved["routes"] == [{"account": "work", "host": "db.invalid", "target": "100.64.1.2"}]
        assert b"db.invalid" in run("work", "list").stdout
        assert run("home", "add", "db.invalid", check=False).returncode != 0
        assert b"502" == run("exec", "curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", "http://db.invalid").stdout
        # A saved forward uses its named account even if the target is reachable
        # directly. A logged-out account must not fall back to the OS network.
        forward_target = f"127.0.0.1:{server.server_port}"
        run("work", "forward", "add", forward_target)
        saved = json.loads(Path(directory, "config.json").read_text())
        forward_port = saved["forwards"][0]["local_port"]
        assert saved["forwards"][0]["target"] == forward_target
        assert b"logged out" in run("work", "forward", "list").stdout
        with socket.create_connection(("127.0.0.1", forward_port), timeout=3) as client:
            try:
                assert client.recv(1) == b""
            except ConnectionResetError:
                pass
        # Legacy on-disk identities survive rename and a daemon restart.
        credentials = Path(directory, "accounts", "work", "credential-fixture")
        credentials.parent.mkdir(parents=True)
        credentials.write_bytes(b"keep-this-identity")
        run("work", "rename", "office")
        saved = json.loads(Path(directory, "config.json").read_text())
        assert saved["accounts"]["office"]["state_dir"] == "work"
        assert saved["routes"][0]["account"] == "office"
        assert saved["forwards"][0]["account"] == "office"
        assert b"keep-this-identity" == credentials.read_bytes()
        assert run("office", "rename", "home", check=False).returncode != 0
        assert run("office", "rename", "status", check=False).returncode != 0
        run("stop")
        for _ in range(50):
            if not Path(directory, "daemon.sock").exists():
                break
            time.sleep(0.1)
        assert b"office" in run("status").stdout
        assert b"db.invalid" in run("office", "list").stdout
        assert f"127.0.0.1:{forward_port}".encode() in run("office", "forward", "list").stdout
        assert b"Cancelled" in run("office", "clear", data=b"n\n").stderr
        assert b"db.invalid" in run("office", "list").stdout
        assert f"127.0.0.1:{forward_port}".encode() in run("office", "forward", "list").stdout
        run("office", "clear", data=b"y\n")
        assert b"No TCP forwards" in run("office", "forward", "list").stdout
        assert b"db.invalid" not in run("office", "list").stdout
        assert b"keep-this-identity" == credentials.read_bytes()
        run("office", "add", "db.invalid")
        run("office", "forward", "add", forward_target)
        legacy_logout = run("office", "logout", data=b"y\n", check=False)
        assert legacy_logout.returncode != 0 and b"--machine-removed" in legacy_logout.stderr
        assert credentials.exists() and b"db.invalid" in run("office", "list").stdout
        assert b"Cancelled" in run("office", "logout", "--machine-removed", data=b"\n").stderr
        assert credentials.exists() and b"db.invalid" in run("office", "list").stdout
        run("office", "logout", "--machine-removed", data=b"y\n")
        saved = json.loads(Path(directory, "config.json").read_text())
        assert not saved.get("forwards") and not saved.get("routes")
        assert b"office" not in run("status").stdout
        assert not credentials.parent.exists()
        url = f"http://127.0.0.1:{server.server_port}/"
        assert run("exec", "curl", "-fsS", url).stdout == b"subscale direct path works"
        request = f"GET / HTTP/1.0\r\nHost: localhost\r\n\r\n".encode()
        assert b"subscale direct path works" in run("connect", "127.0.0.1", str(server.server_port), data=request).stdout
        # PAC is an origin endpoint, so fetch it directly rather than through itself.
        import urllib.request
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        with opener.open("http://" + proxy_address + "/proxy.pac", timeout=3) as response:
            assert b"FindProxyForURL" in response.read()
        run("stop")
        for _ in range(50):
            if not Path(directory, "daemon.sock").exists():
                break
            time.sleep(0.1)
        assert not Path(directory, "daemon.sock").exists()
        print("PASS: concurrent startup, authenticated list, rename persistence, saved TCP forwards, clear and logout, reserved names, account conflicts, fail-closed routing, HTTP, TCP CONNECT, PAC, shutdown")
    finally:
        try:
            run("stop", check=False)
        finally:
            server.shutdown()
            server.server_close()
