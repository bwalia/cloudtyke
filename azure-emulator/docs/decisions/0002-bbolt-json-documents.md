# ADR 0002: bbolt with one JSON document per resource

* Status: accepted (phase 0)

## Context

The prompt suggests embedded SQLite or BoltDB. Static, CGO-free builds and simple prefix queries over ARM IDs matter
more than relational queries.

## Decision

Use `go.etcd.io/bbolt` (pure Go) with buckets of JSON documents, keyed by lower-cased ARM ID (ARM is case-insensitive).
An in-memory implementation of the same interface backs `CLOUDTYKE_EPHEMERAL=true` and the tests. Snapshots are a
JSON dump of every bucket.

## Consequences

* **No CGO**, so the image builds on distroless/static.
* **"Children of scope" is a prefix scan**; that covers cascade delete, resource listing and role assignments at a scope.
* **Queries stay simple.** Anything richer (policy compliance aggregation, KQL-lite) is computed in memory, which is fine
  at emulator scale.
