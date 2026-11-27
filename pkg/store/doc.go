// Package store is the shared, in-process cache of Kubernetes list+watch
// results. One shard per (context, resource type, namespace) is kept up to date
// by a single reflector goroutine and shared by every consumer through leases.
//
// The package has four layers:
//
//   - Source adapters (source_*.go) turn a typed clientset, the dynamic client,
//     the metadata client or a server-side Table request into one list+watch
//     interface. Table and metadata sources fall back to full objects on their
//     own and mark the shard degraded.
//   - Shard (shard.go) is the reflector and the immutable entry map. Entries are
//     never mutated after they are published, so readers need no copying.
//   - Subscriptions (subscribe.go) deliver batched changes through a bounded
//     queue that can never block the reflector.
//   - Manager (manager.go) owns the shards: leases with a linger period, the
//     cluster-wide-serves-namespaced rule, a byte budget with LRU eviction of
//     idle shards, and Peek for readers that must never start a LIST.
//
// Nothing in this package imports the application package.
package store
