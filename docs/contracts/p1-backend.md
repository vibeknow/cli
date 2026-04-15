# P1 backend contract (CLI expectations)

**Status:** DRAFT — awaiting backend sign-off from go-vibeknow / go-figlens / go-account / go-vectoria owners.
**CLI assumed version when implementing:** these shapes; integration tests mock them.

## 1. `X-Vibeknow-Api-Version` header

Every response from every external service includes:

```
X-Vibeknow-Api-Version: v1
```

- CLI compares to its compile-time constant `httpclient.ClientAPIVersion = "v1"`.
- Major mismatch → CLI returns exit 3 (version_mismatch).
- Missing header → CLI logs warning in `--verbose` only; does NOT fail (graceful degradation for services that haven't adopted yet).

## 2. `/v1/health` endpoint

Every external service exposes:

```
GET /v1/health
```

Response body (200 OK):

```json
{
  "status": "ok",
  "version": "<git-sha-or-semver>",
  "api_version": "v1"
}
```

Used by `vibeknow doctor`. Unauthenticated.

## 3. Error response shape

All non-2xx responses (except 204) have body:

```json
{
  "code": 40401,
  "message": "document not found",
  "data": null,
  "trace_id": "tx_abc123",
  "retryable": false
}
```

- `code`: aether error code (int). HTTP status is derived from the upper digits.
- `message`: human-readable.
- `data`: optional extra context (any JSON).
- `trace_id`: for correlation with backend logs.
- `retryable`: **new field**, default false. true for transient failures (upstream timeout, lock conflict, rate-limited).

CLI maps this into spec §11.2 `ErrorObject`:
- `code: "auth_required"` if HTTP 401
- `code: "auth_expired"` if backend code indicates token expiry
- `code: "not_found"` if HTTP 404
- `code: "permission_denied"` if HTTP 403
- `code: "rate_limited"` if HTTP 429
- `code: "internal_error"` if HTTP 5xx
- `code: "unknown"` otherwise
- `retryable` propagates from backend

## 4. Authentication

- All requests include `Authorization: Bearer <jwt>` header.
- JWT is signed by go-account. Every external service verifies via shared go-atlas JWT secret.
- Missing / invalid / expired: HTTP 401 with `code: 40101` (or similar) and `message` indicating the reason.

## 5. go-account endpoints used by P1

- `GET /v1/user/profile` — CLI's `auth whoami` calls this. Returns `{uid, nickname, email, phone, created_at}`.

(OTP/login endpoints such as `/v1/auth/email` etc. exist but P1 does NOT use them — P1.5 scope.)

## 6. What P1 does NOT require yet

- Rate limit response headers (`X-RateLimit-*`) — future.
- Task event streams — P2 prereq.
- Device flow endpoints — P1.5 scope.
