# Loggerbin

A self-hosted paste service with browser-side authenticated encryption. Forked from [Merlin Fuchs' Vaultbin](https://github.com/merlinfuchs/vaultbin), licensed under MIT.

Loggerbin generates a fresh AES-256-GCM key in the browser, encrypts the text before uploading it, and keeps the key in the link fragment (`#key=...`). The server stores ciphertext under an independent random paste ID. It never needs the plaintext or encryption key. It has no third-party scripts, analytics, or plaintext browser history cache.

## Quick start

```sh
git clone https://github.com/LoggeL/loggerbin.git
cd loggerbin
docker compose up -d --build
```

Open http://localhost:8080. Localhost supports Web Crypto. Put an HTTPS reverse proxy in front before sharing the service outside your computer. The Compose port binds only to loopback, and the container runs as UID/GID 65532 with a read-only root filesystem and a persistent private data volume.

## Deploy with Dokploy

Build directly from GitHub. No Docker Hub account or registry login is needed.

1. Create a Dokploy application from this repository's main branch.
2. Choose the Dockerfile build type, with Dockerfile at the repository root and context `.`.
3. Add a named volume mounted at `/data`, keep one replica, and route your HTTPS domain to internal port `8080`.
4. Enable automatic deployments on GitHub pushes. Do not publish port 8080 to the host.

See [the Dokploy setup guide](docs/dokploy.md) for persistence, limits and proxy settings.

## Privacy and security

- Text is encrypted and decrypted in the browser using AES-256-GCM with a fresh 96-bit nonce and a 128-bit authentication tag.
- Share the complete link. Its fragment contains the key. Anyone with that link can read and save the paste.
- HTTP requests, application logs and stored records do not contain the key. Request logs contain fixed route patterns, method and status only.
- Automatic plaintext caching is disabled. Legacy HTMX plaintext snapshots are removed when loading Loggerbin on the same dedicated origin. Plaintext is cleared on page exit and expiry; Back reloads the paste and rechecks the server. Keys can still remain in browser URL history and in the clipboard after you copy a link.
- Expiry denies further server retrieval and reclaims logical storage through cleanup. It cannot erase copies made by recipients, old disk pages, backups or snapshots.
- A host can serve modified JavaScript. Browser encryption does not remove the need to trust the code served by your instance. Use HTTPS and keep the instance updated.
- The server can see ciphertext size, creation/expiry times and normal connection metadata. This is not an anonymity service.

See [SECURITY.md](SECURITY.md) and [docs/hosting.md](docs/hosting.md).

## Configuration

Configuration uses environment variables. There are no built-in passwords.

| Variable | Default | Meaning |
| --- | --- | --- |
| `LOGGERBIN_HOST` | `127.0.0.1` (`0.0.0.0` in the image) | Listen address |
| `LOGGERBIN_PORT` | `8080` | HTTP port |
| `LOGGERBIN_DATA_DIR` | `data` (`/data` in the image) | Private database directory |
| `LOGGERBIN_MAX_PASTE_BYTES` | `65536` | Maximum UTF-8 plaintext bytes and corresponding ciphertext limit; at most 1 MiB |
| `LOGGERBIN_DEFAULT_TTL_SECONDS` | `86400` | Default expiry, 24 hours |
| `LOGGERBIN_MAX_TTL_SECONDS` | `2592000` | Maximum expiry, 30 days; configuration capped at one year |
| `LOGGERBIN_MAX_STORAGE_BYTES` | `67108864` | Total active serialized-record quota |
| `LOGGERBIN_MAX_DATABASE_BYTES` | `268435456` | Hard maximum database-file size, 256 MiB |
| `LOGGERBIN_MAX_PASTES` | `10000` | Total active paste quota |
| `LOGGERBIN_CREATE_RATE` | `0.2` | Per-client creation tokens replenished per second |
| `LOGGERBIN_CREATE_BURST` | `5` | Maximum immediate per-client creation burst |
| `LOGGERBIN_TRUSTED_PROXIES` | empty | Explicit comma-separated proxy CIDRs; trust no forwarded headers by default |
| `LOGGERBIN_BASIC_AUTH_USER` | empty | Optional username protecting UI, assets and API |
| `LOGGERBIN_BASIC_AUTH_PASSWORD` | empty | Optional password, at least 12 bytes; requires the username |

Creation and read routes have per-client limits and a global request limit. Limiter memory is bounded. Oversized bodies, unknown JSON fields, malformed identifiers and overflowing/negative expiry values are rejected. Zero expiry chooses the configured default.

The active storage quota covers serialized records; the separate database-file quota is enforced by bbolt before file growth. The embedded bbolt database reuses pages after cleanup; deleted pages may remain in the file. Use an additional filesystem/container-volume quota if the directory also holds backups or other files. Back up only from a stopped instance or a filesystem snapshot taken with consistent database semantics.

## API

`GET /api/config` returns public limits. `POST /api/pastes` accepts only an encrypted JSON envelope:

```json
{"version":1,"nonce":"base64url-encoded-12-bytes","ciphertext":"base64url-encoded-ciphertext-and-tag","expiration":86400}
```

This is a format example, not a valid encrypted paste. Both encoded fields use canonical unpadded Base64url. The authenticated additional data is the UTF-8 string `loggerbin:v1`. The encrypted plaintext is UTF-8 text, with no JSON wrapper. Generate a random 32-byte key and a new 12-byte nonce for every paste. The key is not an API field.

A successful creation returns HTTP 201 with `id`, `created_at` and `expires_at`. `GET /api/pastes/{id}` returns the encrypted envelope and those timestamps, or HTTP 404 after expiry. The share URL is `/p/{id}#key={base64url-key}`. There is no server-side plaintext/raw endpoint.

## Development

Go 1.27.1 and Node.js 24 or newer are required for the checked workflow. There are no npm dependencies.

```sh
make test
make security
make build
./dist/loggerbin
```

`loggerbin --version` prints the build version; `loggerbin healthcheck` probes the local HTTP health endpoint. Database startup failures exit nonzero. SIGTERM and Ctrl+C trigger a bounded graceful HTTP shutdown and close the database.

## Compatibility

Version 1.0.0 changes the API, share links, configuration and database format. It does not open or decrypt legacy Vaultbin/Badger databases, and old AES-CFB links do not work in Loggerbin. Keep a legacy instance isolated if you need to retrieve existing pastes, then recreate them through Loggerbin. Do not point Loggerbin at a shared data directory. Review and retire legacy logs/backups separately; upgrading cannot remove keys already recorded there.

## License

MIT. The original Vaultbin copyright and license are retained in [LICENSE](LICENSE). Dependency notices are in [THIRD_PARTY_NOTICES](THIRD_PARTY_NOTICES).
