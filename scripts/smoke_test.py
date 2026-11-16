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
# The production proxy address is fixed; do not interfere with an existing daemon.
with socket.socket() as probe:
    try:
        probe.bind(("127.0.0.1", 18765))
    except OSError:
        raise SystemExit("Port 18765 is occupied; stop the existing proxy before this smoke test.")

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
        assert b"18765/proxy.pac" in run("proxy", "pac").stdout
        run("work", "add", "https://DB.invalid:8443/path", "--to", "100.64.1.2")
        saved = json.loads(Path(directory, "config.json").read_text())
        assert saved["routes"] == [{"account": "work", "host": "db.invalid", "target": "100.64.1.2"}]
        assert b"db.invalid" in run("work", "list").stdout
        assert run("home", "add", "db.invalid", check=False).returncode != 0
        assert b"502" == run("exec", "curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", "http://db.invalid").stdout
        run("work", "remove", "db.invalid")
        url = f"http://127.0.0.1:{server.server_port}/"
        assert run("exec", "curl", "-fsS", url).stdout == b"subscale direct path works"
        request = f"GET / HTTP/1.0\r\nHost: localhost\r\n\r\n".encode()
        assert b"subscale direct path works" in run("connect", "127.0.0.1", str(server.server_port), data=request).stdout
        # PAC is an origin endpoint, so fetch it directly rather than through itself.
        import urllib.request
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        with opener.open("http://127.0.0.1:18765/proxy.pac", timeout=3) as response:
            assert b"FindProxyForURL" in response.read()
        run("stop")
        for _ in range(50):
            if not Path(directory, "daemon.sock").exists():
                break
            time.sleep(0.1)
        assert not Path(directory, "daemon.sock").exists()
        print("PASS: concurrent startup, persistence, account conflicts, fail-closed routing, HTTP, TCP CONNECT, PAC, shutdown")
    finally:
        try:
            run("stop", check=False)
        finally:
            server.shutdown()
            server.server_close()
