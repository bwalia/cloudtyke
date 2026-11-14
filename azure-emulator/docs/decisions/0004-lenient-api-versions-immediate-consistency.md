# ADR 0004: lenient api-versions, immediate consistency, delayed operation status

* Status: accepted (phase 1)

## Context

Providers pin many different api-versions, and they change with every release. Real Azure is eventually consistent,
and its long-running operations take a variable amount of time.

## Decision

* **Any well-formed api-version is accepted for an emulated type by default.** `CLOUDTYKE_STRICT_API_VERSIONS=true`
  restricts requests to the versions declared in the registration, and `docs/coverage.md` lists them.
* **Writes are applied immediately and serialised by one lock.** Async operations report `InProgress` until
  `CLOUDTYKE_ASYNC_DELAY` (default 0) or chaos `asyncDelaySeconds` elapses, and always send `Retry-After: 1`.

## Consequences

* **Determinism:** identical runs produce identical state, which labs and CI idempotency checks rely on.
* **Polling paths are still exercised.** Raise the delay to watch Terraform poll.
* **Shape differences between api-versions are not modelled.** Strict mode surfaces when a client uses a version we
  haven't verified.
