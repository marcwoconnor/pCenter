# Vendored Swagger UI

These assets are vendored from [swagger-ui-dist](https://www.npmjs.com/package/swagger-ui-dist) so pCenter's `/api/docs` works on air-gapped deploys without reaching `cdn.jsdelivr.net`. They're `go:embed`-ed into the binary by `openapi.go`.

## Pinned version

**5.32.4** — downloaded 2026-04-24 from jsdelivr.

## Integrity

```
19d1adce81f59e9c37d11fb554b8d96faf23766c668cef5e6b21eee6b9b0e283  swagger-ui.css
82c56ace487aa04c1f92602eb14739cecf600379741eb40c8810ebd03d98a033  swagger-ui-bundle.js
```

## How to upgrade

```bash
VER=5.x.y   # pick from https://www.npmjs.com/package/swagger-ui-dist
cd backend/internal/api/swagger-ui
curl -sSfo swagger-ui.css       https://cdn.jsdelivr.net/npm/swagger-ui-dist@${VER}/swagger-ui.css
curl -sSfo swagger-ui-bundle.js https://cdn.jsdelivr.net/npm/swagger-ui-dist@${VER}/swagger-ui-bundle.js
curl -sSfo LICENSE              https://cdn.jsdelivr.net/npm/swagger-ui-dist@${VER}/LICENSE
sha256sum swagger-ui.css swagger-ui-bundle.js   # update hashes above
```

Commit with a `chore(vendor): bump swagger-ui to X.Y.Z` message.
