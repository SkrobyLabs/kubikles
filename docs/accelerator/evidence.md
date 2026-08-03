# Kubikles Accelerator acceptance evidence

Kubikles Accelerator is referred to here as Accelerator. This generated page records three mandatory gates run serially on one clean source commit. It contains safe summaries, not raw command output.

- Tested commit: `8ea711de249842cf205b52d720b35849f4190529`
- Authority-input SHA-256: `c268039e59c67437bc1f7d0a4e4355dd8d5be6dd9c09fca6872139cb7dfec95f`
- Captured at: `2026-08-03T21:28:16Z`

| Owner | Command | Result | Duration | Raw record |
|---|---|---|---:|---|
| `2fe9439f` | `make test-accelerator-e2e` | pass | 1267646 ms | [run-2fe9439f.json](evidence/run-2fe9439f.json) |
| `4026deaf` | `make test-accelerator-supply-chain` | pass | 109899 ms | [run-4026deaf.json](evidence/run-4026deaf.json) |
| `dd19f7b6` | `make test-accelerator-00-kind` | pass | 58985 ms | [run-dd19f7b6.json](evidence/run-dd19f7b6.json) |

The observed durations describe this run only. They are not performance claims. Runtime clusters do not enforce signatures, SBOMs, provenance, or vulnerability verification.

[Back to the Accelerator overview](../features/accelerator.md)
