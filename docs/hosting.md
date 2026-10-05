# Hosting Loggerbin

Use the Compose configuration or the published image. Keep the HTTP backend reachable only by your HTTPS reverse proxy. The default Compose port binds to 127.0.0.1.

An example Caddy configuration:

```caddyfile
paste.example.com {
    header Strict-Transport-Security "max-age=31536000"
    reverse_proxy 127.0.0.1:8080
}
```

Caddy obtains and renews HTTPS certificates. Replace the example hostname with your domain. If you configure `LOGGERBIN_TRUSTED_PROXIES`, allow only the actual proxy peer addresses. Overwrite forwarded headers at the edge, and never expose a backend port that permits callers to bypass that edge. Without trusted proxy settings, the application safely limits by the direct connection address, so users behind one proxy share its rate budget.

For a private team service, use VPN/SSO access at the proxy or enable both Basic authentication settings. Basic authentication must be used over HTTPS outside localhost. Protect every `/api/` route as well as the UI; hiding only the home page leaves the API accessible.

Supply secrets through a private environment file or your host's secret manager, never the repository or Docker build arguments. The health endpoint intentionally reveals only version and storage readiness and remains unauthenticated for container probes.

The runtime image has no shell or package manager, runs as UID/GID 65532, and writes only to `/data`. A named Docker volume gets the correct ownership from the image. If you bind-mount a directory, set its ownership for UID/GID 65532 first. Mount it privately; use one Loggerbin process per database file.

Set an actual filesystem/volume quota and monitor disk capacity. `LOGGERBIN_MAX_STORAGE_BYTES` and `LOGGERBIN_MAX_PASTES` bound active data, and `LOGGERBIN_MAX_DATABASE_BYTES` imposes a hard database-file bound; bbolt can retain reusable old pages, and expiry does not erase backups. Use encrypted backups and a documented retention policy. Stop the instance for a simple consistent filesystem backup. Restore the full `loggerbin.db` file while the instance is stopped.

Application request logs contain method, fixed route patterns and status. Configure proxy/CDN logs to avoid private query strings or fragments submitted by mistake. Never add analytics or third-party scripts to paste pages. If users report a leak in an older Vaultbin deployment, rotate the leaked secrets themselves; a software upgrade cannot revoke copies already made.

For public anonymous instances, set appropriate quotas, enforce edge connection limits, monitor abuse and establish a removal/reporting policy. Random IDs do not prevent users from deliberately publishing harmful or private content. The operator can remove ciphertext but cannot inspect its contents through this application.
