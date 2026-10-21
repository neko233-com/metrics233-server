# Tempo trace protocol fixtures

`tempo-trace.json` is a synthetic, credential-free contract fixture for deterministic search/detail UI tests. It
exercises Tempo's old and new scope names together, hex/base64 IDs, numeric/string status enums and a nested child.

`tempo-upstream-trace.json` is a reduced excerpt of the first three spans in Grafana Tempo's
[`trace-with-failed-requests.json`](https://github.com/grafana/tempo/blob/d07248b8d00a629e3a076b0e93c57151a3996344/modules/generator/processor/servicegraphs/testdata/trace-with-failed-requests.json)
at commit `d07248b8d00a629e3a076b0e93c57151a3996344`, retrieved 2026-10-21. Trace/span/parent IDs, operation names,
span kinds, nanosecond timestamps, status codes and service names are preserved; unrelated attributes, events and
scopes are omitted. The upstream project is [Grafana Tempo](https://github.com/grafana/tempo), licensed under
[AGPL-3.0](https://github.com/grafana/tempo/blob/d07248b8d00a629e3a076b0e93c57151a3996344/LICENSE).

These are protocol/renderer fixtures. Neither test proves connectivity to a running Tempo instance.
