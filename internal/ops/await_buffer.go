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
func collectBatch(ev <-chan awaitEvent, debounce time.Duration, deadline <-chan time.Time) ([]map[string]any, bool) {
	var batch []map[string]any
	seen := map[int]bool{}

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

	for {
		select {
		case e, ok := <-ev:
			if !ok {
				return batch, batch == nil
			}
			if seen[e.ID] {
				continue
			}
			seen[e.ID] = true
			batch = append(batch, e.Msg)
			arm()
		case <-timer.C:
			if len(batch) > 0 {
				return batch, false
			}
		case <-deadline:
			if len(batch) > 0 {
				return batch, false
			}
			return nil, true
		}
	}
}
