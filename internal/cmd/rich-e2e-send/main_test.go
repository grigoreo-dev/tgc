package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gotd/td/tg"
)

func TestParseOptions(t *testing.T) {
	t.Run("missing target", func(t *testing.T) {
		_, err := parseOptions([]string{"--profile", "e2ebot"})
		if err == nil {
			t.Fatal("expected error for missing --target")
		}
		if !strings.Contains(err.Error(), "target") {
			t.Fatalf("error %q should mention target", err)
		}
	})

	t.Run("defaults and target", func(t *testing.T) {
		opts, err := parseOptions([]string{"--target", "@alice"})
		if err != nil {
			t.Fatalf("parseOptions: %v", err)
		}
		if opts.Target != "@alice" {
			t.Fatalf("Target = %q, want @alice", opts.Target)
		}
		if opts.Profile != "e2ebot" {
			t.Fatalf("Profile = %q, want e2ebot", opts.Profile)
		}
		if opts.Fixture != "internal/markup/testdata/richmessage_alltypes.bin" {
			t.Fatalf("Fixture = %q, want default fixture path", opts.Fixture)
		}
		if opts.MediaDir != "/tmp/demo_media" {
			t.Fatalf("MediaDir = %q, want /tmp/demo_media", opts.MediaDir)
		}
	})

	t.Run("all flags", func(t *testing.T) {
		opts, err := parseOptions([]string{
			"--target", "me",
			"--profile", "p1",
			"--fixture", "/x.bin",
			"--media-dir", "/media",
		})
		if err != nil {
			t.Fatalf("parseOptions: %v", err)
		}
		if opts.Target != "me" || opts.Profile != "p1" || opts.Fixture != "/x.bin" || opts.MediaDir != "/media" {
			t.Fatalf("unexpected options: %+v", opts)
		}
	})
}

func TestMediaPaths(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		dir := t.TempDir()
		// Only create four of five required files.
		for _, name := range []string{"photo_0.jpg", "photo_1.jpg", "photo_2.jpg", "dubaiVideo.mp4"} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		_, err := mediaPaths(dir)
		if err == nil {
			t.Fatal("expected error for missing media file")
		}
		if !strings.Contains(err.Error(), "Neon Rain Train.mp3") {
			t.Fatalf("error %q should mention missing filename", err)
		}
	})

	t.Run("empty file", func(t *testing.T) {
		dir := t.TempDir()
		writeMediaSet(t, dir)
		if err := os.WriteFile(filepath.Join(dir, "photo_1.jpg"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := mediaPaths(dir)
		if err == nil {
			t.Fatal("expected error for empty media file")
		}
		if !strings.Contains(err.Error(), "photo_1.jpg") {
			t.Fatalf("error %q should mention empty filename", err)
		}
	})

	t.Run("non-regular file", func(t *testing.T) {
		dir := t.TempDir()
		writeMediaSet(t, dir)
		// Replace photo_0.jpg with a directory (not a regular file).
		path := filepath.Join(dir, "photo_0.jpg")
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := mediaPaths(dir)
		if err == nil {
			t.Fatal("expected error for non-regular media path")
		}
		if !strings.Contains(err.Error(), "photo_0.jpg") {
			t.Fatalf("error %q should mention non-regular filename", err)
		}
	})

	t.Run("complete directory", func(t *testing.T) {
		dir := t.TempDir()
		writeMediaSet(t, dir)
		paths, err := mediaPaths(dir)
		if err != nil {
			t.Fatalf("mediaPaths: %v", err)
		}
		want := []string{
			"photo_0.jpg",
			"photo_1.jpg",
			"photo_2.jpg",
			"dubaiVideo.mp4",
			"Neon Rain Train.mp3",
		}
		if len(paths) != 5 {
			t.Fatalf("got %d paths, want 5: %v", len(paths), paths)
		}
		for _, name := range want {
			p, ok := paths[name]
			if !ok {
				t.Fatalf("missing key %q in %v", name, paths)
			}
			if p != filepath.Join(dir, name) {
				t.Fatalf("paths[%q] = %q, want %q", name, p, filepath.Join(dir, name))
			}
			st, err := os.Stat(p)
			if err != nil {
				t.Fatalf("stat %s: %v", p, err)
			}
			if !st.Mode().IsRegular() || st.Size() == 0 {
				t.Fatalf("%s not a non-empty regular file", p)
			}
		}
	})
}

func writeMediaSet(t *testing.T, dir string) {
	t.Helper()
	files := map[string][]byte{
		"photo_0.jpg":         []byte("jpg0"),
		"photo_1.jpg":         []byte("jpg1"),
		"photo_2.jpg":         []byte("jpg2"),
		"dubaiVideo.mp4":      []byte("mp4"),
		"Neon Rain Train.mp3": []byte("mp3"),
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSentMessageID(t *testing.T) {
	t.Run("UpdateShortSentMessage", func(t *testing.T) {
		id, err := sentMessageID(&tg.UpdateShortSentMessage{ID: 42})
		if err != nil {
			t.Fatalf("sentMessageID: %v", err)
		}
		if id != 42 {
			t.Fatalf("id = %d, want 42", id)
		}
	})

	t.Run("Updates with UpdateNewMessage", func(t *testing.T) {
		id, err := sentMessageID(&tg.Updates{
			Updates: []tg.UpdateClass{
				&tg.UpdateNewMessage{Message: &tg.Message{ID: 7}},
			},
		})
		if err != nil {
			t.Fatalf("sentMessageID: %v", err)
		}
		if id != 7 {
			t.Fatalf("id = %d, want 7", id)
		}
	})

	t.Run("Updates with UpdateNewChannelMessage", func(t *testing.T) {
		id, err := sentMessageID(&tg.Updates{
			Updates: []tg.UpdateClass{
				&tg.UpdateNewChannelMessage{Message: &tg.Message{ID: 555}},
			},
		})
		if err != nil {
			t.Fatalf("sentMessageID: %v", err)
		}
		if id != 555 {
			t.Fatalf("id = %d, want 555", id)
		}
	})

	t.Run("UpdatesCombined with UpdateNewMessage", func(t *testing.T) {
		id, err := sentMessageID(&tg.UpdatesCombined{
			Updates: []tg.UpdateClass{
				&tg.UpdateNewMessage{Message: &tg.Message{ID: 99}},
			},
		})
		if err != nil {
			t.Fatalf("sentMessageID: %v", err)
		}
		if id != 99 {
			t.Fatalf("id = %d, want 99", id)
		}
	})

	t.Run("UpdatesCombined with UpdateNewChannelMessage", func(t *testing.T) {
		id, err := sentMessageID(&tg.UpdatesCombined{
			Updates: []tg.UpdateClass{
				&tg.UpdateNewChannelMessage{Message: &tg.Message{ID: 1001}},
			},
		})
		if err != nil {
			t.Fatalf("sentMessageID: %v", err)
		}
		if id != 1001 {
			t.Fatalf("id = %d, want 1001", id)
		}
	})

	t.Run("no ID updates", func(t *testing.T) {
		_, err := sentMessageID(&tg.Updates{
			Updates: []tg.UpdateClass{
				&tg.UpdateReadHistoryOutbox{Peer: &tg.PeerUser{UserID: 1}, MaxID: 1},
			},
		})
		if err == nil {
			t.Fatal("expected error when update has no message id")
		}
	})

	t.Run("nil updates", func(t *testing.T) {
		_, err := sentMessageID(nil)
		if err == nil {
			t.Fatal("expected error for nil updates")
		}
	})
}

func TestSendRequest(t *testing.T) {
	ip := &tg.InputPeerUser{UserID: 1, AccessHash: 2}
	rm := &tg.InputRichMessage{
		Blocks: []tg.PageBlockClass{
			&tg.PageBlockParagraph{Text: &tg.TextPlain{Text: "hi"}},
		},
	}
	req := sendRequest(ip, rm)
	if req == nil {
		t.Fatal("sendRequest returned nil")
	}
	if req.Peer != ip {
		t.Fatalf("Peer = %v, want %v", req.Peer, ip)
	}
	if req.Message != "" {
		t.Fatalf("Message = %q, want empty plain text", req.Message)
	}
	if req.RandomID == 0 {
		t.Fatal("RandomID must be non-zero")
	}
	got, ok := req.GetRichMessage()
	if !ok {
		t.Fatal("RichMessage not set")
	}
	if got != rm {
		t.Fatalf("RichMessage = %v, want attached payload", got)
	}
	req2 := sendRequest(ip, rm)
	if req2.RandomID == 0 {
		t.Fatal("second RandomID must be non-zero")
	}
}
