# Deploy on Dokploy

Dokploy builds Loggerbin directly from GitHub. No Docker Hub login is required. The build downloads the public, pinned Go toolchain image; the resulting runtime image is kept on your own server.

## Application setup

Create an application with these settings:

| Setting | Value |
| --- | --- |
| Source | GitHub, your Loggerbin repository |
| Branch and trigger | `main`, pushes |
| Automatic deployment | enabled |
| Build type | Dockerfile |
| Dockerfile | `Dockerfile` |
| Docker context | `.` |
| Build path | `/` |
| Replicas | `1` |
| Update order | stop first |
| Memory limit | `128 MiB` or higher |
| CPU limit | `1` |
| Storage | named volume mounted at `/data` |
| Domain | your hostname, HTTPS enabled |
| Domain target port | `8080` |
| Published host ports | none |

Use a new, dedicated volume such as `loggerbin-data`. Its initial contents and ownership come from the image (UID/GID 65532). A bind mount requires matching ownership. Two replicas must never share the database file. Stop-first updates release the database lock before the replacement starts and cause a brief restart interruption.

Dockerfile health checks run the application binary directly; a shell is not needed. Preserve the data volume across upgrades. The release version defaults to the version in Dockerfile, so manual version build arguments are unnecessary.

The optional `compose.dokploy.yaml` provides the same source build, one instance, persistent storage and a read-only container. If using Dokploy Compose, select this file and add the domain through Dokploy's Domains tab for service `loggerbin`, port `8080`. Do not add a host port.

## Runtime settings

The defaults allow 64 KiB of text per paste, 24-hour default expiry, 30-day maximum expiry, 64 MiB of active records, and a 256 MiB hard database-file limit. Change these through Dokploy's environment settings if needed, within the limits documented in README.md.

Keep `LOGGERBIN_TRUSTED_PROXIES` empty until you have verified the actual proxy network and blocked direct backend access. With the default, clients behind the same proxy share a rate budget. If you later trust forwarded addresses, use only the real proxy peer CIDRs and verify that the edge overwrites untrusted forwarded headers. Do not trust all private networks or every address.

For a team-only instance, put access control at the HTTPS proxy or set both `LOGGERBIN_BASIC_AUTH_USER` and `LOGGERBIN_BASIC_AUTH_PASSWORD` in Dokploy's private environment settings. The password must have at least 12 bytes. Do not put credentials in Git, Dockerfile or build arguments.

Keep one instance per database. Monitor disk capacity and back up the stopped database volume or use a consistent snapshot. Expiry cannot erase recipient copies, old disk pages, or backups. See [hosting.md](hosting.md) for retention and operator responsibilities.

## Verify a deployment

- `/healthz` returns HTTP 200 and the expected release version.
- The domain uses HTTPS, and HTTP redirects to HTTPS.
- Create a synthetic paste in a browser, open its complete link in another tab, and verify the exact text.
- Reload or redeploy the application and check that an unexpired paste survives.
- Check that content security and no-store headers remain present and that private source files are unavailable.
- Confirm the deployment tracks the intended GitHub commit and that only this application's service was changed.

Useful reference: [Dokploy applications](https://docs.dokploy.com/docs/core/applications).
