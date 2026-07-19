package ops

import (
	"testing"
	"time"

	"github.com/gotd/td/tg"
)

func TestParseDateArg(t *testing.T) {
	d, err := ParseDateArg("2026-07-01")
	if err != nil {
		t.Fatal(err)
	}
	if d.Year() != 2026 || d.Month() != 7 || d.Day() != 1 {
		t.Fatalf("got %v", d)
	}
	d2, err := ParseDateArg("2026-07-01T15:04:05Z")
	if err != nil {
		t.Fatal(err)
	}
	if d2.Hour() != 15 {
		t.Fatalf("got %v", d2)
	}
	if _, err := ParseDateArg("yesterday"); err == nil {
		t.Fatal("want error for garbage")
	}
}

func TestMessageToMapBasics(t *testing.T) {
	msg := &tg.Message{
		ID:      42,
		Message: "hi",
		Date:    int(time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC).Unix()),
		FromID:  &tg.PeerUser{UserID: 7},
	}
	msg.SetReplyTo(&tg.MessageReplyHeader{ReplyToMsgID: 41})
	users := map[int64]*tg.User{7: {ID: 7, FirstName: "Vasya", Username: "vasya"}}

	m := messageToMap(msg, users, nil, 100)

	if m["id"] != 42 || m["chat_id"] != int64(100) || m["text"] != "hi" {
		t.Fatalf("basics: %+v", m)
	}
	if m["sender_id"] != int64(7) || m["sender_name"] != "Vasya" || m["sender_username"] != "vasya" {
		t.Fatalf("sender: %+v", m)
	}
	if m["reply_to"] != 41 {
		t.Fatalf("reply_to: %v", m["reply_to"])
	}
	if m["date"] != "2026-07-01T12:00:00Z" {
		t.Fatalf("date: %v", m["date"])
	}
}

func TestMessageToMapDocumentMedia(t *testing.T) {
	doc := &tg.Document{ID: 1, MimeType: "application/pdf", Size: 1234}
	doc.Attributes = []tg.DocumentAttributeClass{&tg.DocumentAttributeFilename{FileName: "report.pdf"}}
	msg := &tg.Message{ID: 1, Date: 0}
	msg.SetMedia(&tg.MessageMediaDocument{Document: doc})

	m := messageToMap(msg, nil, nil, 5)
	media, ok := m["media"].(map[string]any)
	if !ok {
		t.Fatalf("media missing: %+v", m)
	}
	if media["type"] != "document" || media["file_name"] != "report.pdf" ||
		media["mime"] != "application/pdf" || media["size"] != int64(1234) {
		t.Fatalf("media: %+v", media)
	}
}

func TestMessageToMapPhotoMedia(t *testing.T) {
	msg := &tg.Message{ID: 2}
	msg.SetMedia(&tg.MessageMediaPhoto{Photo: &tg.Photo{ID: 9}})
	m := messageToMap(msg, nil, nil, 5)
	media := m["media"].(map[string]any)
	if media["type"] != "photo" {
		t.Fatalf("media: %+v", media)
	}
}

func TestMessageToMapGroupedID(t *testing.T) {
	// An album member carries grouped_id; expose it so callers can
	// reassemble albums (jq group_by(.grouped_id)).
	grouped := &tg.Message{ID: 10}
	grouped.SetGroupedID(123456789)
	m := messageToMap(grouped, nil, nil, 5)
	if m["grouped_id"] != int64(123456789) {
		t.Fatalf("grouped_id: want 123456789, got %v (%+v)", m["grouped_id"], m)
	}

	// A non-grouped message must NOT emit the key at all (no clutter).
	single := &tg.Message{ID: 11}
	m2 := messageToMap(single, nil, nil, 5)
	if _, present := m2["grouped_id"]; present {
		t.Fatalf("grouped_id must be absent for non-grouped message: %+v", m2)
	}
}
