# Remote Preview Sharing

Share zerobased-proxied services with remote reviewers. The machine must be reachable via a tunnel — zerobased handles the routing, the tunnel handles connectivity and TLS.

## How It Works

1. Tunnel exposes Caddy's port 80 to the internet (or tailnet)
2. `zerobased domain add <domain>` registers routes for all services on that domain
3. Caddy matches incoming `Host` headers and proxies to the right container
4. Wildcard DNS on the tunnel side resolves `*.domain` to the tunnel endpoint

## Tailscale Setup

One-time setup:
```bash
# Expose Caddy to tailnet (private, only tailnet members)
tailscale serve --bg --https=443 80

# Or expose to public internet via Funnel
tailscale funnel --bg --https=443 80
```

Register domain in zerobased:
```bash
# Auto-detect tailscale hostname
zerobased domain add $(tailscale status --json | jq -r .Self.DNSName | sed 's/\.$//')

# Example result:
# @1  dev-box.tail1234.ts.net  (3h59m remaining)
```

Reviewer accesses: `https://api-3000.myapp.dev-box.tail1234.ts.net`

### Tailscale Properties

| Aspect | Detail |
|--------|--------|
| DNS | Automatic via MagicDNS (wildcard subdomains supported) |
| TLS | Automatic via Tailscale |
| Auth | Tailnet-private by default; public via Funnel |
| Wildcard hosts | Supported since Tailscale 1.52+ |
| Reviewer needs client | Yes for Serve, No for Funnel |
| Latency | Peer-to-peer (fastest option) |

## Cloudflare Tunnel Setup

One-time setup:
```bash
# Create tunnel
cloudflared tunnel create dev-preview

# Configure wildcard ingress in ~/.cloudflared/config.yml:
tunnel: <tunnel-id>
ingress:
  - hostname: "*.preview.yourdomain.com"
    service: http://localhost:80
  - service: http_status:404

# Add wildcard DNS CNAME in Cloudflare dashboard:
# *.preview.yourdomain.com → <tunnel-id>.cfargotunnel.com

# Start tunnel
cloudflared tunnel run dev-preview
```

Register domain in zerobased:
```bash
zerobased domain add preview.yourdomain.com
```

Reviewer accesses: `https://api-3000.myapp.preview.yourdomain.com`

### Cloudflare Properties

| Aspect | Detail |
|--------|--------|
| DNS | Requires wildcard CNAME → tunnel ID |
| TLS | Automatic via Cloudflare edge |
| Auth | Public by default; restrict via Cloudflare Access |
| Wildcard hosts | Supported via wildcard DNS record |
| Reviewer needs client | No |
| Latency | Via Cloudflare edge (slightly higher than P2P) |

## Comparison

| | Tailscale | Cloudflare Tunnel |
|--|-----------|-------------------|
| Best for | Team-internal previews | Public/client-facing previews |
| Setup | `tailscale serve 80` | Tunnel + wildcard DNS + domain |
| Auth | Tailnet (automatic) | Public or Cloudflare Access |
| Latency | Peer-to-peer (fastest) | Via Cloudflare edge |
| Domain cost | Free (*.ts.net) | Requires owned domain |
| Zero-config for reviewer | Only with Funnel | Yes |

## Fallback: nip.io (No Tunnel)

For machines with a directly reachable IP (VPS, LAN):
```bash
zerobased domain add 10.0.1.50.nip.io
```

Routes: `api-3000.myapp.10.0.1.50.nip.io` resolves to `10.0.1.50:80`.
No tunnel, no DNS setup. HTTP only (no TLS).

## Both Tunnels Simultaneously

```bash
tailscale serve --bg --https=443 80
cloudflared tunnel run dev-preview

zerobased domain add preview.yourdomain.com         # @1
zerobased domain add $(tailscale status --json | jq -r .Self.DNSName | sed 's/\.$//') # @2

zerobased share @1    # Cloudflare URLs
zerobased share @2    # Tailscale URLs
```

## TTL Behavior

- Default: 4 hours — share freely, auto-cleans
- Custom: `--ttl 30m`, `--ttl 8h`
- Persistent: `--persistent` — never expires
- Expired domains are swept every 30 seconds by the daemon health loop
- `zerobased unshare --all` for immediate cleanup

## Path-Based Routing with Tunnels

If the tunnel doesn't support wildcard DNS, use a routefile for path-based routing instead:

```
# zerobased.routes
/api     api
/        frontend
```

Single hostname `myapp.preview.dev.co` routes by path:
- `myapp.preview.dev.co/api` → API container
- `myapp.preview.dev.co/` → frontend container

No wildcard DNS needed.
