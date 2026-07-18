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
	batch, timedOut := collectBatch(ev, 100*time.Millisecond, deadline)
	if timedOut {
		t.Fatal("did not expect timeout")
	}
	if len(batch) != 3 {
		t.Fatalf("want 3 messages, got %d", len(batch))
	}
}

func TestCollectBatchTimeoutEmpty(t *testing.T) {
	ev := make(chan awaitEvent)
	deadline := time.After(50 * time.Millisecond)
	batch, timedOut := collectBatch(ev, time.Second, deadline)
	if !timedOut || batch != nil {
		t.Fatalf("want (nil,true), got (%v,%v)", batch, timedOut)
	}
}

func TestCollectBatchDeadlineFlushesNonEmpty(t *testing.T) {
	ev := make(chan awaitEvent, 4)
	deadline := time.After(60 * time.Millisecond)
	go func() { send(ev, 1, "late") }() // arrives, then deadline fires during debounce
	batch, timedOut := collectBatch(ev, 500*time.Millisecond, deadline)
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
	batch, _ := collectBatch(ev, 80*time.Millisecond, deadline)
	if len(batch) != 2 {
		t.Fatalf("want 2 after dedup, got %d", len(batch))
	}
}

func TestCollectBatchNoDebounce(t *testing.T) {
	// debounce=0 means "return as soon as something is buffered".
	ev := make(chan awaitEvent, 4)
	deadline := time.After(5 * time.Second)
	send(ev, 1, "immediate")
	batch, timedOut := collectBatch(ev, 0, deadline)
	if timedOut || len(batch) != 1 {
		t.Fatalf("debounce=0: want 1 msg no timeout, got %d timedOut=%v", len(batch), timedOut)
	}
}
