#!/usr/bin/env python3
"""Remote HTTP MCP server backed by the local Aegis API.

The default transport is MCP Streamable HTTP (JSON-RPC over ``POST /mcp``).
The stdio transport remains available for local development with
``--transport stdio``.  Logs always go to stderr so stdio clients receive
only JSON-RPC on stdout.
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import re
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any
from urllib.error import HTTPError, URLError
from urllib.parse import urlencode
from urllib.request import Request, urlopen


SERVER_NAME = "aegis-local-mcp"
SERVER_VERSION = "0.2.0"
MCP_PROTOCOL_VERSION = "2025-11-25"
SUPPORTED_PROTOCOLS = {"2024-11-05", "2025-03-26", "2025-06-18", "2025-11-25"}
DEFAULT_HOST = "127.0.0.1"
DEFAULT_PORT = 8085
MCP_PATH = "/mcp"
LOGGER = logging.getLogger("aegis_mcp")
UUID_RE = re.compile(
    r"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-"
    r"[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$"
)


class MCPError(Exception):
    """An expected tool or JSON-RPC error."""


class AegisAPIError(MCPError):
    """An expected failure while calling api-server."""


def _validate_host_id(host_id: str) -> str:
    host_id = host_id.strip()
    if not UUID_RE.fullmatch(host_id):
        raise MCPError("host_id must be a valid UUID")
    return host_id


def _configure_logging() -> None:
    level_name = os.getenv("AEGIS_MCP_LOG_LEVEL", "INFO").upper()
    level = getattr(logging, level_name, logging.INFO)
    logging.basicConfig(
        level=level,
        stream=sys.stderr,
        format="%(asctime)s %(levelname)s aegis-mcp %(message)s",
    )


class AegisClient:
    def __init__(self) -> None:
        self.base_url = os.getenv("AEGIS_API_URL", "http://127.0.0.1:8082").rstrip("/")
        try:
            self.timeout = max(1.0, min(float(os.getenv("AEGIS_API_TIMEOUT", "10")), 60.0))
        except ValueError:
            self.timeout = 10.0

    def _token(self) -> str:
        token = os.getenv("AEGIS_API_TOKEN", "").strip()
        if token:
            return token
        token_file = os.getenv("AEGIS_API_TOKEN_FILE", "").strip()
        if token_file:
            try:
                token = Path(token_file).expanduser().read_text(encoding="utf-8").strip()
            except OSError as exc:
                raise AegisAPIError(f"cannot read AEGIS_API_TOKEN_FILE: {exc}") from exc
            if token:
                return token
        raise AegisAPIError(
            "Aegis authentication is not configured; set AEGIS_API_TOKEN or AEGIS_API_TOKEN_FILE"
        )

    def _get(self, path: str, query: dict[str, str] | None = None, authenticated: bool = True) -> Any:
        url = f"{self.base_url}{path}"
        if query:
            url += "?" + urlencode(query)
        headers = {"Accept": "application/json"}
        if authenticated:
            headers["Authorization"] = f"Bearer {self._token()}"
        LOGGER.debug("aegis_api_request path=%s authenticated=%s", path, authenticated)
        request = Request(url, method="GET", headers=headers)
        try:
            with urlopen(request, timeout=self.timeout) as response:
                payload = response.read()
        except HTTPError as exc:
            if exc.code in (401, 403):
                raise AegisAPIError(f"Aegis authentication rejected the request ({exc.code})") from exc
            raise AegisAPIError(f"Aegis API returned HTTP {exc.code}") from exc
        except URLError as exc:
            raise AegisAPIError(f"cannot reach Aegis API: {exc.reason}") from exc
        except TimeoutError as exc:
            raise AegisAPIError("Aegis API request timed out") from exc
        try:
            decoded = json.loads(payload.decode("utf-8"))
        except (UnicodeDecodeError, json.JSONDecodeError) as exc:
            raise AegisAPIError("Aegis API returned invalid JSON") from exc
        if isinstance(decoded, dict) and decoded.get("code") not in (None, 0):
            raise AegisAPIError(str(decoded.get("message", "Aegis API request failed"))[:300])
        return decoded

    def health(self) -> dict[str, Any]:
        payload = self._get("/health", authenticated=False)
        return payload if isinstance(payload, dict) else {"status": "unknown"}

    def list_hosts(self, query: str = "") -> dict[str, Any]:
        query = query.strip()
        if len(query) > 256:
            raise MCPError("query must be at most 256 characters")
        payload = self._get("/api/v1/hosts", {"query": query} if query else None)
        items = payload.get("data", []) if isinstance(payload, dict) else []
        if not isinstance(items, list):
            raise AegisAPIError("Aegis returned an unexpected host list")
        return {"items": items, "total": len(items), "query": query}

    def get_host(self, host_id: str) -> dict[str, Any]:
        host_id = _validate_host_id(host_id)
        payload = self._get(f"/api/v1/hosts/{host_id}")
        data = payload.get("data") if isinstance(payload, dict) else None
        if not isinstance(data, dict):
            raise AegisAPIError("Aegis returned an unexpected host detail")
        return data


TOOLS = [
    {
        "name": "get_aegis_health",
        "title": "Aegis health",
        "description": "Read the local Aegis API health status. This operation is read-only and requires no Aegis session token.",
        "inputSchema": {"type": "object", "properties": {}, "additionalProperties": False},
    },
    {
        "name": "list_hosts",
        "title": "List Aegis hosts",
        "description": "List hosts registered in Aegis. Optionally filter by hostname or IP substring. Read-only.",
        "inputSchema": {
            "type": "object",
            "properties": {"query": {"type": "string", "description": "Optional hostname or IP substring."}},
            "additionalProperties": False,
        },
    },
    {
        "name": "get_host",
        "title": "Get Aegis host",
        "description": "Get one registered Aegis host by UUID, including its online status. Read-only.",
        "inputSchema": {
            "type": "object",
            "properties": {"host_id": {"type": "string", "description": "Aegis host UUID."}},
            "required": ["host_id"],
            "additionalProperties": False,
        },
    },
]


def _json_text(value: Any) -> str:
    return json.dumps(value, ensure_ascii=False, indent=2, sort_keys=True)


def _tool_result(value: Any, is_error: bool = False) -> dict[str, Any]:
    return {"content": [{"type": "text", "text": _json_text(value)}], "isError": is_error}


def _negotiate_protocol(requested: Any) -> str:
    return requested if isinstance(requested, str) and requested in SUPPORTED_PROTOCOLS else MCP_PROTOCOL_VERSION


def handle_request(message: dict[str, Any], client: AegisClient) -> dict[str, Any] | None:
    method = message.get("method")
    request_id = message.get("id")
    is_notification = "id" not in message
    if method in {"notifications/initialized", "notifications/cancelled", "notifications/progress"}:
        return None
    if method == "initialize":
        params = message.get("params") or {}
        result = {
            "protocolVersion": _negotiate_protocol(params.get("protocolVersion")),
            "capabilities": {"tools": {"listChanged": False}},
            "serverInfo": {"name": SERVER_NAME, "version": SERVER_VERSION},
            "instructions": "Use get_aegis_health, list_hosts, or get_host. Host inventory tools are read-only.",
        }
        return None if is_notification else {"jsonrpc": "2.0", "id": request_id, "result": result}
    if method == "ping":
        return None if is_notification else {"jsonrpc": "2.0", "id": request_id, "result": {}}
    if method == "tools/list":
        return None if is_notification else {"jsonrpc": "2.0", "id": request_id, "result": {"tools": TOOLS}}
    if method == "tools/call":
        if is_notification:
            return None
        name = "unknown"
        try:
            params = message.get("params") or {}
            name = params.get("name")
            arguments = params.get("arguments") or {}
            if not isinstance(name, str) or not name:
                raise MCPError("tool name is required")
            if not isinstance(arguments, dict):
                raise MCPError("tool arguments must be an object")
            if name == "get_aegis_health":
                if arguments:
                    raise MCPError("unknown arguments: " + ", ".join(sorted(arguments)))
                value = client.health()
            elif name == "list_hosts":
                unknown = set(arguments) - {"query"}
                if unknown:
                    raise MCPError(f"unknown arguments: {', '.join(sorted(unknown))}")
                value = client.list_hosts(str(arguments.get("query", "")))
            elif name == "get_host":
                unknown = set(arguments) - {"host_id"}
                if unknown:
                    raise MCPError(f"unknown arguments: {', '.join(sorted(unknown))}")
                value = client.get_host(_validate_host_id(str(arguments.get("host_id", ""))))
            else:
                raise MCPError(f"unknown tool: {name}")
            return {"jsonrpc": "2.0", "id": request_id, "result": _tool_result(value)}
        except MCPError as exc:
            LOGGER.warning("mcp_tool_failed tool=%s reason=%s", name, exc)
            return {"jsonrpc": "2.0", "id": request_id, "result": _tool_result(str(exc), True)}
    if is_notification:
        return None
    return {"jsonrpc": "2.0", "id": request_id, "error": {"code": -32601, "message": f"method not found: {method}"}}


def _configured_access_token() -> str:
    return os.getenv("AEGIS_MCP_ACCESS_TOKEN", "").strip()


class MCPHTTPHandler(BaseHTTPRequestHandler):
    server_version = "AegisMCP/0.2"

    def _send_json(self, payload: dict[str, Any], status: int = 200) -> None:
        body = json.dumps(payload, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Cache-Control", "no-store")
        self.end_headers()
        self.wfile.write(body)

    def _authorized(self) -> bool:
        expected = _configured_access_token()
        if not expected:
            return True
        return self.headers.get("Authorization", "") == f"Bearer {expected}"

    def do_GET(self) -> None:  # noqa: N802
        if self.path == "/health":
            self._send_json({"status": "ok", "server": SERVER_NAME, "version": SERVER_VERSION})
            return
        if self.path == MCP_PATH:
            self._send_json({"error": "MCP endpoint requires POST"}, 405)
            return
        self._send_json({"error": "not found"}, 404)

    def do_POST(self) -> None:  # noqa: N802
        if self.path != MCP_PATH:
            self._send_json({"error": "not found"}, 404)
            return
        if not self._authorized():
            self._send_json({"error": "unauthorized"}, 401)
            return
        try:
            length = int(self.headers.get("Content-Length", "0"))
        except ValueError:
            length = 0
        if length <= 0 or length > 1_048_576:
            self._send_json({"jsonrpc": "2.0", "id": None, "error": {"code": -32600, "message": "invalid request size"}}, 400)
            return
        try:
            message = json.loads(self.rfile.read(length).decode("utf-8"))
            if not isinstance(message, dict):
                raise ValueError("JSON-RPC message must be an object")
            response = handle_request(message, self.server.aegis_client)  # type: ignore[attr-defined]
        except (json.JSONDecodeError, UnicodeDecodeError, ValueError) as exc:
            LOGGER.warning("mcp_http_parse_failed reason=%s", exc)
            self._send_json({"jsonrpc": "2.0", "id": None, "error": {"code": -32700, "message": "parse error"}}, 400)
            return
        if response is not None:
            self._send_json(response)

    def log_message(self, fmt: str, *args: Any) -> None:
        LOGGER.info("mcp_http_request " + fmt, *args)


def run_http(host: str, port: int, client: AegisClient) -> int:
    server = ThreadingHTTPServer((host, port), MCPHTTPHandler)
    server.aegis_client = client  # type: ignore[attr-defined]
    LOGGER.info("mcp_http_started host=%s port=%d path=%s", host, port, MCP_PATH)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        LOGGER.info("mcp_http_stopping")
    finally:
        server.server_close()
    return 0


def run_stdio(client: AegisClient) -> int:
    for line in sys.stdin:
        if not line.strip():
            continue
        try:
            message = json.loads(line)
            if not isinstance(message, dict):
                raise ValueError("JSON-RPC message must be an object")
            response = handle_request(message, client)
        except (json.JSONDecodeError, ValueError) as exc:
            LOGGER.warning("mcp_stdio_parse_failed reason=%s", exc)
            response = {"jsonrpc": "2.0", "id": None, "error": {"code": -32700, "message": "parse error"}}
        if response is not None:
            sys.stdout.write(json.dumps(response, ensure_ascii=False, separators=(",", ":")) + "\n")
            sys.stdout.flush()
    return 0


def main(argv: list[str] | None = None) -> int:
    _configure_logging()
    parser = argparse.ArgumentParser(description="Aegis remote MCP server")
    parser.add_argument("--transport", choices=("http", "stdio"), default=os.getenv("AEGIS_MCP_TRANSPORT", "http"))
    parser.add_argument("--host", default=os.getenv("AEGIS_MCP_HOST", DEFAULT_HOST))
    parser.add_argument("--port", type=int, default=int(os.getenv("AEGIS_MCP_PORT", str(DEFAULT_PORT))))
    args = parser.parse_args(argv)
    client = AegisClient()
    if args.transport == "stdio":
        return run_stdio(client)
    return run_http(args.host, args.port, client)


if __name__ == "__main__":
    raise SystemExit(main())
