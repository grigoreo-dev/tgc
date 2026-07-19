// internal/ops/await_buffer_test.go
package ops

import (
	"testing"
	"time"
)

func send(ch chan awaitEvent, id int, text string) {
	ch <- awaitEvent{ID: id, Msg: map[string]any{"id": id, "text": text}}
}

func TestCollectBatchCoalescesBurst(t *testing.T) {
	ev := make(chan awaitEvent, 8)
	deadline := time.After(5 * time.Second)
	go func() {
		send(ev, 1, "a")
		time.Sleep(20 * time.Millisecond)
		send(ev, 2, "b")
		time.Sleep(20 * time.Millisecond)
		send(ev, 3, "c")
	}()
	batch, lastID, timedOut := collectBatch(ev, nil, 100*time.Millisecond, deadline)
	if timedOut {
		t.Fatal("did not expect timeout")
	}
	if len(batch) != 3 {
		t.Fatalf("want 3 messages, got %d", len(batch))
	}
	if lastID != 3 {
		t.Fatalf("want lastID 3, got %d", lastID)
	}
}

func TestCollectBatchTimeoutEmpty(t *testing.T) {
	ev := make(chan awaitEvent)
	deadline := time.After(50 * time.Millisecond)
	batch, _, timedOut := collectBatch(ev, nil, time.Second, deadline)
	if !timedOut || batch != nil {
		t.Fatalf("want (nil,true), got (%v,%v)", batch, timedOut)
	}
}

func TestCollectBatchDeadlineFlushesNonEmpty(t *testing.T) {
	ev := make(chan awaitEvent, 4)
	deadline := time.After(60 * time.Millisecond)
	go func() { send(ev, 1, "late") }() // arrives, then deadline fires during debounce
	batch, _, timedOut := collectBatch(ev, nil, 500*time.Millisecond, deadline)
	if timedOut {
		t.Fatal("non-empty buffer must flush, not timeout")
	}
	if len(batch) != 1 {
		t.Fatalf("want 1, got %d", len(batch))
	}
}

func TestCollectBatchDedup(t *testing.T) {
	ev := make(chan awaitEvent, 4)
	deadline := time.After(5 * time.Second)
	go func() {
		send(ev, 7, "x")
		send(ev, 7, "x-dup")
		send(ev, 8, "y")
	}()
	batch, _, _ := collectBatch(ev, nil, 80*time.Millisecond, deadline)
	if len(batch) != 2 {
		t.Fatalf("want 2 after dedup, got %d", len(batch))
	}
}

func TestCollectBatchNoDebounce(t *testing.T) {
	// debounce=0 means "return as soon as something is buffered".
	ev := make(chan awaitEvent, 4)
	deadline := time.After(5 * time.Second)
	send(ev, 1, "immediate")
	batch, _, timedOut := collectBatch(ev, nil, 0, deadline)
	if timedOut || len(batch) != 1 {
		t.Fatalf("debounce=0: want 1 msg no timeout, got %d timedOut=%v", len(batch), timedOut)
	}
}

func TestCollectBatchPreseedAndDedup(t *testing.T) {
	// The startup drain is delivered via preseed (never through ev), so even a
	// large drain cannot be dropped by a full channel. Live events dedup against
	// preseeded ids, and lastID reflects the max across both.
	ev := make(chan awaitEvent, 2)
	deadline := time.After(5 * time.Second)
	preseed := []awaitEvent{
		{ID: 1, Msg: map[string]any{"id": 1}},
		{ID: 2, Msg: map[string]any{"id": 2}},
		{ID: 3, Msg: map[string]any{"id": 3}},
	}
	go func() {
		send(ev, 3, "dup-of-preseed") // must be deduped
		send(ev, 4, "live")
	}()
	batch, lastID, timedOut := collectBatch(ev, preseed, 80*time.Millisecond, deadline)
	if timedOut {
		t.Fatal("preseed present: must not time out")
	}
	if len(batch) != 4 {
		t.Fatalf("want 4 (3 preseed + 1 new live, dup dropped), got %d", len(batch))
	}
	if lastID != 4 {
		t.Fatalf("want lastID 4, got %d", lastID)
	}
}
