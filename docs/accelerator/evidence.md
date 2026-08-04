# Kubikles Accelerator acceptance evidence

Kubikles Accelerator is referred to here as Accelerator. This generated page records three mandatory gates run serially on one clean source commit. It contains safe summaries, not raw command output.

- Tested commit: `d1657d3a98fdd838b13f41b31c52a72c1d28af25`
- Authority-input SHA-256: `50a6254ac672a5e10ff20cea2db8742f7f92c057a087a3c9e59eb1d74029abce`
- Captured at: `2026-08-04T11:00:19Z`

| Owner | Command | Result | Duration | Raw record |
|---|---|---|---:|---|
| `2fe9439f` | `make test-accelerator-e2e` | pass | 1095913 ms | [run-2fe9439f.json](evidence/run-2fe9439f.json) |
| `4026deaf` | `make test-accelerator-supply-chain` | pass | 147731 ms | [run-4026deaf.json](evidence/run-4026deaf.json) |
| `dd19f7b6` | `make test-accelerator-00-kind` | pass | 58068 ms | [run-dd19f7b6.json](evidence/run-dd19f7b6.json) |

The observed durations describe this run only. They are not performance claims. Runtime clusters do not enforce signatures, SBOMs, provenance, or vulnerability verification.

[Back to the Accelerator overview](../features/accelerator.md)
