# Embedded HNSW core

Graph and heap are derived from [coder/hnsw v0.6.1](https://github.com/coder/hnsw/tree/v0.6.1), distributed under
the accompanying CC0-1.0 license. This narrow in-tree adaptation is maintained with the vector store.

The upstream package's file persistence depends on `renameio.TempFile`, which does not compile on Windows.
Only the graph/heap core is included; durable payloads and indexes belong to our transactional Badger store.
The adaptation uses standard-library maps, scalar Float64-accumulated distances with explicit zero-vector
behavior, and the configured metric during deletion/replenishment instead of a hard-coded cosine metric.
Numeric graph keys keep neighbor identity storage bounded. No platform-specific persistence or SIMD dependency
is included. Persistence, nearest-neighbor recall, filters and deletion require tests at the owning-store layer.

Layer search retains the configured `EfSearch` beam until the best remaining candidate is worse than its
worst retained neighbor. A local plateau cannot discard queued bridges. The min-heap's last leaf is not
assumed to be its maximum: bounded pruning scans the beam for the true maximum, and returned candidates
are sorted by distance and key. Bridge/pruning regressions and repeated store-level recall checks cover this.
