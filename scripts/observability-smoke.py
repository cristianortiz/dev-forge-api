#!/usr/bin/env python3
"""Verify that a fresh synthetic signal reaches each local backend."""

import base64
import json
import os
from pathlib import Path
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid


ROOT = Path(__file__).resolve().parent.parent
GRAFANA = os.environ.get("OBS_GRAFANA_URL", "http://127.0.0.1:3000").rstrip("/")
USER = os.environ.get("OBS_GRAFANA_USER", "admin")
PASSWORD = os.environ.get("OBS_GRAFANA_PASSWORD", "admin")
SERVICE = "obs-smoke-" + uuid.uuid4().hex


def query(path, params=None):
    url = GRAFANA + path
    if params:
        url += "?" + urllib.parse.urlencode(params)
    auth = base64.b64encode(f"{USER}:{PASSWORD}".encode()).decode()
    request = urllib.request.Request(url, headers={"Authorization": "Basic " + auth})
    with urllib.request.urlopen(request, timeout=5) as response:
        data = json.load(response)
    if isinstance(data, dict) and data.get("status") == "error":
        raise RuntimeError(f"Backend query failed: {data.get('error', data)}")
    return data


def generate(signal):
    subprocess.run(
        [
            "docker", "compose", "-f", str(ROOT / "deployments/docker-compose.yml"),
            "run", "--rm", "--no-deps", "telemetrygen", signal,
            "--otlp-http", "--otlp-insecure", "--otlp-endpoint", "otel-lgtm:4318",
            "--service", SERVICE, "--" + signal, "1", "--rate", "0",
        ],
        cwd=ROOT,
        check=True,
    )


def main():
    print(f"Sending synthetic telemetry for {SERVICE}", flush=True)
    datasources = query("/api/datasources")
    uids = {item["type"]: item["uid"] for item in datasources}
    for kind in ("tempo", "prometheus", "loki"):
        if kind not in uids:
            raise RuntimeError(f"Grafana datasource not found: {kind}")

    for signal in ("traces", "metrics", "logs"):
        generate(signal)

    proxy = "/api/datasources/proxy/uid/"
    checks = {
        "traces": (
            proxy + uids["tempo"] + "/api/search",
            {"q": '{ resource.service.name = "' + SERVICE + '" }', "limit": 10},
            lambda data: any(t.get("rootServiceName") == SERVICE for t in data.get("traces", [])),
        ),
        "metrics": (
            proxy + uids["prometheus"] + "/api/v1/query",
            {"query": 'gen{job="' + SERVICE + '"}'},
            lambda data: bool(data.get("data", {}).get("result")),
        ),
        "logs": (
            proxy + uids["loki"] + "/loki/api/v1/query_range",
            {"query": '{service_name="' + SERVICE + '"}', "limit": 10},
            lambda data: bool(data.get("data", {}).get("result")),
        ),
    }
    deadline = time.monotonic() + 120
    last_errors = {}
    while checks and time.monotonic() < deadline:
        for signal, (path, params, found) in list(checks.items()):
            try:
                if found(query(path, params)):
                    print(f"OK: {signal} for {SERVICE}", flush=True)
                    del checks[signal]
                else:
                    last_errors[signal] = "signal not found"
            except (urllib.error.URLError, TimeoutError, ValueError, RuntimeError) as err:
                last_errors[signal] = str(err)
        if checks:
            time.sleep(2)
    if checks:
        raise RuntimeError("Timed out: " + "; ".join(
            f"{signal}: {last_errors.get(signal, 'not checked')}" for signal in checks
        ))
    print("All three synthetic signals reached their backends.")


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, RuntimeError, subprocess.CalledProcessError) as error:
        print(f"Observability smoke test failed: {error}", file=sys.stderr)
        sys.exit(1)
