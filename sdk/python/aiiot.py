"""aiiot platform SDK (generated from docs/openapi.yaml style contract).

Minimal, dependency-free client (standard library only). Covers authentication,
projects/workspaces/products/thing models/devices, telemetry (latest, query,
compare, CSV export), device certificates, rules/alerts and notification
channels.

Example:
    from aiiot import Client

    c = Client("http://localhost:8080")
    c.login("admin", "secret")
    devices = c.devices(project_id=1)
    rows = c.telemetry(device_id=devices[0]["id"], identifier="temperature",
                       interval="5m", from_="2026-10-01T00:00:00Z")
"""
from __future__ import annotations

import json
import urllib.error
import urllib.parse
import urllib.request
from typing import Any, Optional


class ApiError(Exception):
    """Raised for non-2xx responses. `.status` carries the HTTP code and
    `.body` the decoded server error message when available."""

    def __init__(self, status: int, body: str):
        super().__init__(f"HTTP {status}: {body}")
        self.status = status
        self.body = body


class Client:
    base: str
    token: str = ""

    def __init__(self, base_url: str = "http://localhost:8080", timeout: float = 15.0):
        self.base = base_url.rstrip("/")
        self.timeout = timeout

    # --- auth -----------------------------------------------------------------
    def login(self, username: str, password: str) -> str:
        data = self._post("/api/v1/auth/login", {"username": username, "password": password})
        self.token = data["token"]
        return self.token

    # --- projects / workspaces ------------------------------------------------
    def projects(self) -> list[dict]:
        return self._get("/api/v1/projects")

    def create_project(self, name: str, key: str = "", description: str = "") -> dict:
        return self._post("/api/v1/projects", {"name": name, "key": key, "description": description})

    def workspaces(self, project_id: int) -> list[dict]:
        return self._get(f"/api/v1/projects/{project_id}/workspaces")

    def create_workspace(self, project_id: int, name: str, key: str = "") -> dict:
        return self._post(f"/api/v1/projects/{project_id}/workspaces", {"name": name, "key": key})

    # --- products & thing models ------------------------------------------------
    def products(self) -> list[dict]:
        return self._get("/api/v1/products")

    def create_product(self, name: str, protocol: str = "mqtt", key: str = "",
                       description: str = "") -> dict:
        return self._post("/api/v1/products",
                          {"name": name, "protocol": protocol, "key": key, "description": description})

    def thing_model(self, product_id: int) -> dict:
        return self._get(f"/api/v1/products/{product_id}/thing-model")

    def save_thing_model(self, product_id: int, elements: list[dict], version: str = "",
                         description: str = "") -> dict:
        return self._put(f"/api/v1/products/{product_id}/thing-model",
                         {"version": version, "description": description, "elements": elements})

    def publish_thing_model(self, product_id: int) -> dict:
        return self._post(f"/api/v1/products/{product_id}/thing-model/publish", None)

    # --- devices ----------------------------------------------------------------
    def devices(self, project_id: int, **query) -> list[dict]:
        return self._get(f"/api/v1/projects/{project_id}/devices", query)

    def create_device(self, product_id: int, name: str, workspace_id: int,
                      key: str = "", secret: str = "", tags: Optional[dict] = None) -> dict:
        return self._post(f"/api/v1/products/{product_id}/devices",
                          {"key": key, "name": name, "workspaceId": workspace_id,
                           "secret": secret, "tags": tags or {}})

    def device(self, device_id: int) -> dict:
        return self._get(f"/api/v1/devices/{device_id}")

    def export_devices_csv(self, product_id: int, path: str) -> None:
        raw = self._raw("GET", f"/api/v1/products/{product_id}/devices/export")
        with open(path, "wb") as fh:
            fh.write(raw)

    def import_devices_csv(self, product_id: int, path: str) -> dict:
        boundary = "----aiiot-sdk"
        with open(path, "rb") as fh:
            content = fh.read()
        payload = (f"--{boundary}\r\n"
                   f'Content-Disposition: form-data; name="file"; filename="devices.csv"\r\n'
                   f"Content-Type: text/csv\r\n\r\n").encode()
        payload += content + f"\r\n--{boundary}--\r\n".encode()
        return self._raw_json("POST", f"/api/v1/products/{product_id}/devices/import",
                              payload, content_type=f"multipart/form-data; boundary={boundary}")

    def issue_certificate(self, device_id: int) -> dict:
        return self._post(f"/api/v1/devices/{device_id}/certificate", None)

    def revoke_certificate(self, device_id: int) -> dict:
        return self._raw_json("DELETE", f"/api/v1/devices/{device_id}/certificate")

    def certificate_status(self, device_id: int) -> dict:
        return self._get(f"/api/v1/devices/{device_id}/certificate")

    # --- telemetry ---------------------------------------------------------------
    def latest(self, device_id: int) -> list[dict]:
        return self._get(f"/api/v1/devices/{device_id}/latest")

    def telemetry(self, device_id: int, identifier: str, interval: str = "",
                  from_: str = "", to: str = "", limit: int = 0) -> list[dict]:
        q = {"identifier": identifier}
        if interval:
            q["interval"] = interval
        if from_:
            q["from"] = from_
        if to:
            q["to"] = to
        if limit:
            q["limit"] = str(limit)
        data = self._get(f"/api/v1/devices/{device_id}/telemetry", q)
        return data.get("points", [])

    def compare(self, device_ids: list[int], identifiers: list[str], interval: str = "5m",
                from_: str = "", to: str = "") -> list[dict]:
        body: dict[str, Any] = {"deviceIds": device_ids, "identifiers": identifiers,
                                "interval": interval}
        if from_:
            body["from"] = from_
        if to:
            body["to"] = to
        return self._post("/api/v1/telemetry/compare", body).get("series", [])

    def export_telemetry_csv(self, device_id: int, identifiers: list[str], path: str,
                             interval: str = "", from_: str = "", to: str = "") -> None:
        q = {"identifiers": ",".join(identifiers)}
        if interval:
            q["interval"] = interval
        if from_:
            q["from"] = from_
        if to:
            q["to"] = to
        raw = self._raw("GET", f"/api/v1/devices/{device_id}/telemetry/export", q)
        with open(path, "wb") as fh:
            fh.write(raw)

    # --- rules / alerts / channels --------------------------------------------------
    def rules(self, project_id: int) -> list[dict]:
        return self._get(f"/api/v1/projects/{project_id}/rules")

    def alerts(self, project_id: int, **query) -> list[dict]:
        return self._get(f"/api/v1/projects/{project_id}/alerts", query)

    def channels(self, project_id: int) -> list[dict]:
        return self._get(f"/api/v1/projects/{project_id}/channels")

    def create_channel(self, project_id: int, name: str, channel_type: str,
                       config: Optional[dict] = None, enabled: bool = True) -> dict:
        return self._post(f"/api/v1/projects/{project_id}/channels",
                          {"name": name, "type": channel_type, "config": config or {},
                           "enabled": enabled})

    def test_channel(self, channel_id: int, title: str = "aiiot SDK test",
                     body: str = "Hello from the aiiot SDK") -> dict:
        return self._post(f"/api/v1/channels/{channel_id}/test",
                          {"title": title, "body": body})

    # --- transport ------------------------------------------------------------
    def _get(self, path: str, query: Optional[dict] = None) -> Any:
        return self._raw_json("GET", path, query=query)

    def _post(self, path: str, body: Optional[dict]) -> Any:
        return self._raw_json("POST", path, body=body if body is not None else {})

    def _put(self, path: str, body: dict) -> Any:
        return self._raw_json("PUT", path, body=body)

    def _raw_json(self, method: str, path: str, body=None, query=None,
                  content_type: str = "application/json") -> Any:
        payload = None
        if body is not None and content_type == "application/json":
            payload = json.dumps(body).encode()
        raw = self._raw(method, path, payload, query, content_type)
        if not raw:
            return {}
        try:
            return json.loads(raw)
        except json.JSONDecodeError:
            return raw.decode()

    def _raw(self, method: str, path: str, payload=None, query=None,
             content_type: str = "application/json") -> bytes:
        url = self.base + path
        if query:
            url += "?" + urllib.parse.urlencode({k: v for k, v in query.items() if v})
        headers = {}
        if payload is not None:
            headers["Content-Type"] = content_type
        if self.token:
            headers["Authorization"] = f"Bearer {self.token}"
        req = urllib.request.Request(url, data=payload, headers=headers, method=method)
        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                return resp.read()
        except urllib.error.HTTPError as e:
            raw = e.read().decode(errors="replace")
            raise ApiError(e.code, raw) from e