# Non-streaming cluster access

Polling mode is stored per kubeconfig context and preserves streaming as the
default elsewhere. It replaces resource watches, event refresh and log follow
with completion-scheduled requests. A shared per-context budget permits at most
four concurrent background polling operations, preventing broad views from
creating unbounded request fan-out.

Log polling retrieves at most 4 MiB per Kubernetes log response and drains at
most five 200-line pages per interval. Remaining pages continue on the next
interval, so a noisy workload cannot trap the UI in an unbounded catch-up loop.

Kubernetes log pagination exposes only a timestamp cursor. If more than one
page of distinct lines has exactly the same nanosecond timestamp, the API
cannot address the remainder independently; Kubikles stops when the cursor no
longer advances rather than looping. This rare loss boundary is preferable to
unbounded memory or request use and should be considered when investigating an
extremely bursty workload.

An eventless watch closure raises the compatibility warning only when the
stream closes within five seconds. Normal long-lived API-server watch expiry
reconnects without being classified as a non-streaming transport.
