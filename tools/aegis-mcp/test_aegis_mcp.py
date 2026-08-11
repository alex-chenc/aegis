#!/usr/bin/env python3

import json
import os
import threading
import unittest
from unittest.mock import patch
from urllib.request import Request, urlopen

from aegis_mcp import AegisAPIError, AegisClient, MCPHTTPHandler, ThreadingHTTPServer, handle_request


class FakeClient:
    def __init__(self):
        self.calls = []

    def list_hosts(self, query=""):
        self.calls.append(("list_hosts", query))
        return {"items": [{"hostname": "host-a"}], "total": 1, "query": query}

    def health(self):
        self.calls.append(("health",))
        return {"status": "ok"}

    def get_host(self, host_id):
        self.calls.append(("get_host", host_id))
        return {"id": host_id, "hostname": "host-a", "online": True}


class FakeHTTPResponse:
    status = 200

    def __init__(self, payload):
        self.payload = payload

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc_value, traceback):
        return False

    def read(self):
        return json.dumps(self.payload).encode("utf-8")


class MCPServerTests(unittest.TestCase):
    def test_initialize_and_tools_list(self):
        client = FakeClient()
        response = handle_request(
            {
                "jsonrpc": "2.0",
                "id": 1,
                "method": "initialize",
                "params": {"protocolVersion": "2025-03-26"},
            },
            client,
        )
        self.assertEqual(response["result"]["protocolVersion"], "2025-03-26")

        response = handle_request(
            {"jsonrpc": "2.0", "id": 2, "method": "tools/list"}, client
        )
        self.assertEqual(
            [tool["name"] for tool in response["result"]["tools"]],
            ["get_aegis_health", "list_hosts", "get_host"],
        )

    def test_http_streamable_endpoint_handles_initialize(self):
        server = ThreadingHTTPServer(("127.0.0.1", 0), MCPHTTPHandler)
        server.aegis_client = FakeClient()
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            body = json.dumps(
                {
                    "jsonrpc": "2.0",
                    "id": "init-1",
                    "method": "initialize",
                    "params": {"protocolVersion": "2025-06-18"},
                }
            ).encode("utf-8")
            request = Request(
                f"http://127.0.0.1:{server.server_port}/mcp",
                data=body,
                headers={"Content-Type": "application/json", "Accept": "application/json"},
                method="POST",
            )
            with urlopen(request, timeout=2) as response:
                payload = json.loads(response.read().decode("utf-8"))
            self.assertEqual(payload["result"]["protocolVersion"], "2025-06-18")
        finally:
            server.shutdown()
            server.server_close()

    def test_list_hosts_calls_read_only_client(self):
        client = FakeClient()
        response = handle_request(
            {
                "jsonrpc": "2.0",
                "id": 3,
                "method": "tools/call",
                "params": {"name": "list_hosts", "arguments": {"query": "host"}},
            },
            client,
        )
        self.assertFalse(response["result"]["isError"])
        self.assertEqual(client.calls, [("list_hosts", "host")])
        self.assertIn("host-a", response["result"]["content"][0]["text"])

    def test_health_tool_does_not_require_aegis_token(self):
        client = FakeClient()
        response = handle_request(
            {"jsonrpc": "2.0", "id": 6, "method": "tools/call", "params": {"name": "get_aegis_health", "arguments": {}}},
            client,
        )
        self.assertFalse(response["result"]["isError"])
        self.assertEqual(client.calls, [("health",)])

    def test_get_host_rejects_invalid_uuid_before_client_call(self):
        client = FakeClient()
        response = handle_request(
            {
                "jsonrpc": "2.0",
                "id": 4,
                "method": "tools/call",
                "params": {"name": "get_host", "arguments": {"host_id": "../../etc/passwd"}},
            },
            client,
        )
        self.assertTrue(response["result"]["isError"])
        self.assertEqual(client.calls, [])

    def test_api_error_is_reported_as_tool_error(self):
        class BrokenClient(FakeClient):
            def list_hosts(self, query=""):
                raise AegisAPIError("Aegis API is unavailable")

        response = handle_request(
            {
                "jsonrpc": "2.0",
                "id": 5,
                "method": "tools/call",
                "params": {"name": "list_hosts", "arguments": {}},
            },
            BrokenClient(),
        )
        self.assertTrue(response["result"]["isError"])
        self.assertIn("unavailable", response["result"]["content"][0]["text"])

    def test_api_client_uses_bearer_token_and_parses_host_list(self):
        captured = {}

        def fake_urlopen(request, timeout):
            captured["url"] = request.full_url
            captured["authorization"] = request.get_header("Authorization")
            captured["timeout"] = timeout
            return FakeHTTPResponse({"code": 0, "data": [{"hostname": "host-a"}]})

        with patch.dict(
            os.environ,
            {
                "AEGIS_API_URL": "http://aegis.test:8082",
                "AEGIS_API_TOKEN": "test-token",
                "AEGIS_API_TOKEN_FILE": "",
            },
        ), patch("aegis_mcp.urlopen", fake_urlopen):
            result = AegisClient().list_hosts("host")

        self.assertEqual(result["total"], 1)
        self.assertEqual(captured["url"], "http://aegis.test:8082/api/v1/hosts?query=host")
        self.assertEqual(captured["authorization"], "Bearer test-token")
        self.assertEqual(captured["timeout"], 10.0)


if __name__ == "__main__":
    unittest.main()
