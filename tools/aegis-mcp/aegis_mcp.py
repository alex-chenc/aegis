#!/usr/bin/env python3
"""Local stdio MCP server for read-only Aegis host queries.

The process communicates with Codex over newline-delimited JSON-RPC on stdout.
Operational logs are deliberately written to stderr so stdout remains a valid
MCP transport.
"""

from __future__ import annotations

import json
import logging
import os
import re
import sys
from pathlib import Path
from typing import Any
from urllib.error import HTTPError, URLError
from urllib.parse import urlencode
from urllib.request import Request, urlopen


SERVER_NAME = "aegis-hosts-mcp"
SERVER_VERSION = "0.1.0"
MCP_PROTOCOL_VERSION = "2025-06-18"
LOGGER = logging.getLogger("aegis_mcp")
UUID_RE = re.compile(
    r"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-"
    r"[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$"
)


def _validate_host_id(host_id: str) -> str:
    host_id = host_id.strip()
    if not UUID_RE.fullmatch(host_id):
        raise MCPError("host_id must be a valid UUID")
    return host_id


class MCPError(Exception):
    """An expected tool or JSON-RPC error."""


class AegisAPIError(MCPError):
    """An expected failure while calling api-server."""


def _configure_logging() -> None:
    level_name = os.getenv("AEGIS_MCP_LOG_LEVEL", "WARNING").upper()
    level = getattr(logging, level_name, logging.WARNING)
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
            "Aegis authentication is not configured; set AEGIS_API_TOKEN "
            "or AEGIS_API_TOKEN_FILE"
        )

    def _get(self, path: str, query: dict[str, str] | None = None) -> Any:
        token = self._token()
        url = f"{self.base_url}{path}"
        if query:
            url += "?" + urlencode(query)

        LOGGER.debug("aegis_api_request path=%s", path)

        request = Request(
            url,
            method="GET",
            headers={
                "Accept": "application/json",
                "Authorization": f"Bearer {token}",
            },
        )
        try:
            with urlopen(request, timeout=self.timeout) as response:
                payload = response.read()
                LOGGER.debug("aegis_api_response path=%s status=%s", path, response.status)
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
            message = str(decoded.get("message", "Aegis API request failed"))
            raise AegisAPIError(message[:300])
        return decoded

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
        "name": "list_hosts",
        "description": (
            "List hosts registered in Aegis. Optionally filter by a substring "
            "of hostname or IP address. This operation is read-only."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {
                "query": {
                    "type": "string",
                    "description": "Optional hostname or IP substring to search for.",
                }
            },
            "additionalProperties": False,
        },
    },
    {
        "name": "get_host",
        "description": "Get one registered Aegis host by UUID, including online status.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "host_id": {
                    "type": "string",
                    "description": "Aegis host UUID.",
                }
            },
            "required": ["host_id"],
            "additionalProperties": False,
        },
    },
]


def _json_text(value: Any) -> str:
    return json.dumps(value, ensure_ascii=False, indent=2, sort_keys=True)


def _tool_result(value: Any, is_error: bool = False) -> dict[str, Any]:
    return {
        "content": [{"type": "text", "text": _json_text(value)}],
        "isError": is_error,
    }


def _negotiate_protocol(requested: Any) -> str:
    if isinstance(requested, str) and requested:
        if requested in {"2024-11-05", "2025-03-26", "2025-06-18"}:
            return requested
    return MCP_PROTOCOL_VERSION


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
            "instructions": "Use list_hosts or get_host to query the read-only Aegis host inventory.",
        }
        return None if is_notification else {"jsonrpc": "2.0", "id": request_id, "result": result}

    if method == "ping":
        return None if is_notification else {"jsonrpc": "2.0", "id": request_id, "result": {}}

    if method == "tools/list":
        result = {"tools": TOOLS}
        return None if is_notification else {"jsonrpc": "2.0", "id": request_id, "result": result}

    if method == "tools/call":
        if is_notification:
            return None
        try:
            params = message.get("params") or {}
            name = params.get("name")
            arguments = params.get("arguments") or {}
            if not isinstance(name, str) or not name:
                raise MCPError("tool name is required")
            if not isinstance(arguments, dict):
                raise MCPError("tool arguments must be an object")

            if name == "list_hosts":
                unknown = set(arguments) - {"query"}
                if unknown:
                    raise MCPError(f"unknown arguments: {', '.join(sorted(unknown))}")
                value = client.list_hosts(str(arguments.get("query", "")))
            elif name == "get_host":
                unknown = set(arguments) - {"host_id"}
                if unknown:
                    raise MCPError(f"unknown arguments: {', '.join(sorted(unknown))}")
                host_id = _validate_host_id(str(arguments.get("host_id", "")))
                value = client.get_host(host_id)
            else:
                raise MCPError(f"unknown tool: {name}")
            return {"jsonrpc": "2.0", "id": request_id, "result": _tool_result(value)}
        except MCPError as exc:
            LOGGER.warning("mcp_tool_failed tool=%s reason=%s", name, exc)
            return {"jsonrpc": "2.0", "id": request_id, "result": _tool_result(str(exc), True)}

    if is_notification:
        return None
    return {
        "jsonrpc": "2.0",
        "id": request_id,
        "error": {"code": -32601, "message": f"method not found: {method}"},
    }


def main() -> int:
    _configure_logging()
    client = AegisClient()
    for line in sys.stdin:
        if not line.strip():
            continue
        try:
            message = json.loads(line)
            if not isinstance(message, dict):
                raise ValueError("JSON-RPC message must be an object")
            response = handle_request(message, client)
        except (json.JSONDecodeError, ValueError) as exc:
            logging.getLogger(__name__).warning("invalid JSON-RPC input: %s", exc)
            response = {
                "jsonrpc": "2.0",
                "id": None,
                "error": {"code": -32700, "message": "parse error"},
            }
        if response is not None:
            sys.stdout.write(json.dumps(response, ensure_ascii=False, separators=(",", ":")) + "\n")
            sys.stdout.flush()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
