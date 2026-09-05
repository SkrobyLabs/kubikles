# Non-streaming cluster access

The resource toolbar provides a refresh button and a refresh mode menu:
Watcher (the default), Manual, or Every 5/10/30/60 seconds. Mode and interval
are remembered separately for each kubeconfig context. Manual stops automatic
updates; the refresh button and refresh shortcut still reload the current view
and active details, events, and logs.

Polling replaces resource watches with completion-scheduled requests. One
shared clock uses the selected interval after the previous refresh completes.
A shared per-context budget permits at most
four concurrent background polling operations, preventing broad views from
creating unbounded request fan-out.

Followed logs use a separate approximately five-second cadence in polling mode;
the resource interval does not change log follow speed. Manual stops log follow
requests as well. Log polling reads complete lines with a 4 MiB page budget and drains at
most five 200-line pages per interval. Remaining pages continue on the next
interval, so a noisy workload cannot trap the UI in an unbounded catch-up loop.
An oversized page reports an error without advancing the cursor, rather than
silently losing the remainder of a truncated line.

Kubernetes log pagination exposes only a timestamp cursor. If more than one
page of distinct lines has exactly the same nanosecond timestamp, the API
cannot address the remainder independently; Kubikles stops when the cursor no
longer advances rather than looping. This rare loss boundary is preferable to
unbounded memory or request use and should be considered when investigating an
extremely bursty workload.

Five transport failures of the same watch within 60 seconds automatically switch
the context to ten-second polling and show one nonblocking notification. Short
eventless closures (under five seconds) count as failures. Separate resource or
namespace watches never share a failure count, and log failures do not trigger
resource fallback. Failure history expires; opening an HTTP connection alone
does not reset it. Normal long-lived API-server watch expiry reconnects without
being classified as a non-streaming transport. Select Watcher to retry watching.
