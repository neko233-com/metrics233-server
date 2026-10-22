# Embedded HNSW core

Graph and heap are derived from [coder/hnsw v0.6.1](https://github.com/coder/hnsw/tree/v0.6.1), distributed under
the accompanying CC0-1.0 license. This narrow in-tree adaptation is maintained with the vector store.

The upstream package's file persistence depends on `renameio.TempFile`, which does not compile on Windows.
Only the graph/heap core is included; durable payloads and indexes belong to our transactional Badger store.
The adaptation uses standard-library maps, scalar Float64-accumulated distances with explicit zero-vector
behavior, and the configured metric during deletion/replenishment instead of a hard-coded cosine metric.
Numeric graph keys keep neighbor identity storage bounded. No platform-specific persistence or SIMD dependency
is included. Persistence, nearest-neighbor recall, filters and deletion require tests at the owning-store layer.
