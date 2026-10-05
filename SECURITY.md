# Security

Supported releases: the latest Loggerbin 1.x release. Legacy Vaultbin versions are not maintained by this fork.

Report vulnerabilities privately using this repository's GitHub Security Advisories if private reporting is available. If it is not enabled, open an issue asking for a private reporting channel without including exploit details or private data.

## Security boundary

The browser creates AES-256-GCM ciphertext with a 32-byte random key, a 12-byte fresh nonce, a 128-bit tag and authenticated additional data `loggerbin:v1`. The API accepts ciphertext only. The database stores no encryption keys or plaintext. Paste IDs are independent 128-bit random values; the decryption key lives in the URL fragment and is not sent in HTTP requests.

A recipient or browser extension can retain plaintext or the link. A malicious host can replace the browser JavaScript. HTTPS, correct proxy configuration and reviewed release updates remain necessary. Storage expiry is not secure physical erasure. No independent third-party security certification is claimed.

## Repairs from Vaultbin

1. Removed server-side AES-CFB decryption and the Badger borrowed-buffer mutation path.
2. Replaced key-bearing URL paths with independent IDs and fragment-only browser keys.
3. Removed plaintext HTMX history snapshots and inline/evaluated script flows.
4. Added authenticated browser encryption, a strict CSP and safe text-only rendering.
5. Enforced request, ciphertext, metadata, storage, count and rate bounds across the API.
6. Validated expiry as integer seconds before any duration multiplication.
7. Trust forwarded client addresses only through explicitly configured direct proxy peers.
8. Added HTTP timeouts, privacy headers, cleanup and graceful storage shutdown.
9. Removed orphaned view counters and legacy libraries; added security regression tests.
10. Added a pinned minimal non-root container, pinned CI actions and dependency updates.

## Release checks

Run `make test` and `make security`, then perform browser round trips, tamper/expiry checks and non-root container smoke tests. Run a secret scan before publishing. Check the image's platform manifests and verify the remote Git commit and image digest after publication. A vulnerability scanner result is one check, not a guarantee that no vulnerabilities exist.
