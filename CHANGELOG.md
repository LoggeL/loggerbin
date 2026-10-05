# Changelog

## 1.1.0 (2026-10-05)

- A minimal light interface based on an ImageGen design, with a focused editor and compact sharing controls.
- Readable paste sizes, clearer expiry labels and responsive controls for small screens.
- Direct GitHub-to-Dokploy deployment instructions with persistent private storage.
- Container builds run without registry authentication; the Docker Hub publishing workflow is removed.

## 1.0.0 (2026-10-05)

Initial Loggerbin release, forked from Vaultbin v0.2.0.

- Browser-side AES-256-GCM; keys stay in URL fragments and the server stores ciphertext only.
- Independent paste IDs, strict API validation and bounded expiry, storage and request rates.
- A new responsive, accessible paste editor and viewer without third-party browser libraries.
- Privacy headers, a strict CSP, no plaintext history cache, and safe text rendering.
- Private bbolt storage, expiry cleanup, atomic quotas and graceful shutdown.
- Go 1.27.1, security regression tests, pinned CI and a non-root scratch container.

This major release intentionally changes the database format, links, API and configuration. See the compatibility notes in README.md before retiring a legacy instance.
