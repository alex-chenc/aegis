# Frontend 8081 502 Troubleshooting Design V5.5

## Background

External access to `http://34.174.207.156:8081/login` reports 502. Packet capture
on the host shows inbound TCP traffic to port 8081, so the first confirmed fact is
that traffic can reach the machine. It does not prove that the frontend container
accepts the connection, that Nginx returns a valid HTTP response, or that an
upstream dependency is healthy.

The V5.5 deployment exposes the Vue frontend through Nginx:

```text
client -> host:8081 -> aegis-frontend:80 -> static SPA assets
                                  |
                                  +-> /api/* -> aegis-api-server:8082
```

## Problem Analysis

`/login` is a Vue Router route and must be served by the frontend SPA fallback to
`index.html`. It must not depend on `api-server`.

The current Nginx config proxies `/health` to `aegis-api-server:8082`. This makes a
frontend health probe depend on the API service. If a load balancer, reverse proxy,
or operator uses `/health` to decide whether port 8081 is healthy, an API outage can
turn the frontend entrypoint unhealthy and surface as a 502 even though the static
frontend is capable of serving `/login`.

When diagnosing public access, distinguish network reachability from application
reachability:

- Seeing an inbound SYN in `tcpdump` proves the packet reached the host NIC.
- It does not prove Docker DNAT forwarded the flow to `aegis-frontend`.
- It does not prove Nginx accepted the request or wrote an access log entry.
- If `curl http://localhost:8081/login` returns 200 but public-IP curl times out
  and Nginx has no access log for the request, the remaining fault domain is host
  firewall, cloud firewall, routing, NAT, or a public-IP hairpin limitation.
- For Docker-published ports, `net.ipv4.ip_forward` must be `1`. If it is `0`,
  public traffic can reach the host NIC but fail before it is forwarded to the
  frontend container.

## Design

1. Keep SPA routing behavior:
   - `/login`, `/hosts`, and other Vue routes resolve to `/index.html`.
   - Static assets continue to be cached.
2. Make frontend health local to the frontend container:
   - `GET /health` returns `200 ok` from Nginx itself.
   - It must not use `proxy_pass`.
3. Keep API proxying explicit:
   - Only `/api/*` proxies to `aegis-api-server:8082`.
4. Update the Docker healthcheck to use the local frontend health endpoint.

## Verification

Tests first:

1. Add a Vitest test that parses `frontend/nginx.conf`.
2. Assert that `/health` is implemented with a local `return 200` response.
3. Assert that the `/health` location does not proxy to `aegis-api-server`.
4. Assert that `/api/` still proxies to `aegis-api-server:8082`.
5. Assert that `/` keeps SPA fallback behavior for `/login`.

Runtime verification:

1. Build frontend with `npm run build`.
2. If Docker is available, build/start the frontend according to
   `aegis-build-test`.
3. Use `curl`:
   - `curl -i http://localhost:8081/health` should return `200`.
   - `curl -i http://localhost:8081/login` should return `200` and HTML.
   - `curl -i http://localhost:8081/api/...` remains proxied to API.
4. Check container and host routing:
   - `docker compose ps frontend api-server` should show healthy containers.
   - `ss -tlnp | grep 8081` should show Docker listening on `0.0.0.0:8081`.
   - `iptables -t nat -S DOCKER` should include a DNAT rule for `--dport 8081`.
   - `sysctl net.ipv4.ip_forward` should return `net.ipv4.ip_forward = 1`.

Host-level remediation when public traffic reaches the NIC but the browser gets no
response:

```bash
sysctl -w net.ipv4.ip_forward=1
printf 'net.ipv4.ip_forward = 1\n' > /etc/sysctl.d/99-aegis-docker-forward.conf
sysctl --system
```
