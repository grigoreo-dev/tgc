// internal/ops/await_buffer.go
package ops

import "time"

// awaitEvent is one accepted inbound message reduced to what the buffer needs.
type awaitEvent struct {
	ID  int            // tg message id (dedup key)
	Msg map[string]any // already messageToMap-rendered
}

// collectBatch is single-goroutine: the debounce timer is created, armed,
// drained, and read only by this function's own loop. That sole-ownership is
// what makes the Stop/drain/Reset sequence in arm() safe (no concurrent timer
// access). The dispatcher callback never touches the timer — it only sends on ev.
//
// preseed carries the startup-drain events (oldest→newest). They are folded
// into the batch (and dedup set) directly, never through ev, so an unbounded
// number of drained messages cannot be dropped by a full channel. A non-empty
// preseed arms the debounce immediately, so a burst that is fully covered by
// the drain still returns once quiet rather than blocking until the deadline.
//
// It returns the rendered batch, the max message id in the batch (0 when
// empty), and whether it timed out with nothing.
func collectBatch(ev <-chan awaitEvent, preseed []awaitEvent, debounce time.Duration, deadline <-chan time.Time) ([]map[string]any, int, bool) {
	var batch []map[string]any
	seen := map[int]bool{}
	lastID := 0

	add := func(e awaitEvent) bool {
		if seen[e.ID] {
			return false
		}
		seen[e.ID] = true
		batch = append(batch, e.Msg)
		if e.ID > lastID {
			lastID = e.ID
		}
		return true
	}

	// timer starts stopped; armed on first event.
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()

	arm := func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(debounce)
	}

	for _, e := range preseed {
		add(e)
	}
	if len(batch) > 0 {
		arm()
	}

	for {
		select {
		case e, ok := <-ev:
			if !ok {
				return batch, lastID, batch == nil
			}
			if add(e) {
				arm()
			}
		case <-timer.C:
			if len(batch) > 0 {
				return batch, lastID, false
			}
		case <-deadline:
			if len(batch) > 0 {
				return batch, lastID, false
			}
			return nil, 0, true
		}
	}
}
