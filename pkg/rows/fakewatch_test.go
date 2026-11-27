package rows

import "k8s.io/apimachinery/pkg/watch"

// The fake clientset hands every watcher a buffered channel and panics with "channel full" when a
// test writes faster than the watcher is read. Integration tests here move a few hundred objects
// in a tight loop, and under the race detector on a loaded machine the store's reader can fall
// more than the default buffer behind. A real API server applies back-pressure instead, so the
// buffer is sized to hold the largest burst any test makes.
func init() {
	watch.DefaultChanSize = 4096
}
