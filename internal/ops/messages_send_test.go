package ops

import (
	"testing"
	"time"

	"github.com/gotd/td/tg"
)

func TestSentResultFromShortSentMessage(t *testing.T) {
	date := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	upd := &tg.UpdateShortSentMessage{ID: 42, Date: int(date.Unix())}

	res := sentResult(upd, 1234)

	if res["message_id"] != 42 {
		t.Fatalf("message_id = %v, want 42", res["message_id"])
	}
	if res["chat_id"] != int64(1234) {
		t.Fatalf("chat_id = %v, want 1234", res["chat_id"])
	}
	if got := res["date"]; got != date.Format(time.RFC3339) {
		t.Fatalf("date = %v, want %v", got, date.Format(time.RFC3339))
	}
}

func TestSentResultFromUpdates(t *testing.T) {
	date := time.Date(2025, 6, 7, 8, 9, 10, 0, time.UTC)
	upd := &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateNewMessage{
				Message: &tg.Message{ID: 7, Date: int(date.Unix())},
			},
		},
	}

	res := sentResult(upd, 99)

	if res["message_id"] != 7 {
		t.Fatalf("message_id = %v, want 7", res["message_id"])
	}
	if res["chat_id"] != int64(99) {
		t.Fatalf("chat_id = %v, want 99", res["chat_id"])
	}
	if got := res["date"]; got != date.Format(time.RFC3339) {
		t.Fatalf("date = %v, want %v", got, date.Format(time.RFC3339))
	}
}

func TestSentResultFromChannelUpdates(t *testing.T) {
	date := time.Date(2024, 3, 3, 3, 3, 3, 0, time.UTC)
	upd := &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateNewChannelMessage{
				Message: &tg.Message{ID: 555, Date: int(date.Unix())},
			},
		},
	}

	res := sentResult(upd, -100777)

	if res["message_id"] != 555 {
		t.Fatalf("message_id = %v, want 555", res["message_id"])
	}
	if res["chat_id"] != int64(-100777) {
		t.Fatalf("chat_id = %v, want -100777", res["chat_id"])
	}
}

func TestSentResultFromEditMessage(t *testing.T) {
	date := time.Date(2026, 7, 15, 23, 7, 44, 0, time.UTC)
	upd := &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateEditMessage{
				Message: &tg.Message{ID: 953, Date: int(date.Unix())},
			},
		},
	}

	res := sentResult(upd, 42)

	if res["message_id"] != 953 {
		t.Fatalf("message_id = %v, want 953", res["message_id"])
	}
	if res["chat_id"] != int64(42) {
		t.Fatalf("chat_id = %v, want 42", res["chat_id"])
	}
	if got := res["date"]; got != date.Format(time.RFC3339) {
		t.Fatalf("date = %v, want %v", got, date.Format(time.RFC3339))
	}
}

func TestSentResultFromEditChannelMessage(t *testing.T) {
	date := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	upd := &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateEditChannelMessage{
				Message: &tg.Message{ID: 8080, Date: int(date.Unix())},
			},
		},
	}

	res := sentResult(upd, -100500)

	if res["message_id"] != 8080 {
		t.Fatalf("message_id = %v, want 8080", res["message_id"])
	}
	if res["chat_id"] != int64(-100500) {
		t.Fatalf("chat_id = %v, want -100500", res["chat_id"])
	}
}

func TestRandomIDNonZeroAndDistinct(t *testing.T) {
	a := randomID()
	b := randomID()
	if a == 0 || b == 0 {
		t.Fatalf("randomID returned zero: a=%d b=%d", a, b)
	}
	if a == b {
		t.Fatalf("two randomID calls returned the same value: %d", a)
	}
}
