package richfixture

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

func fixturePath(name string) string {
	return filepath.Join("..", "markup", "testdata", name)
}

func TestDecodeAllTypesFixture(t *testing.T) {
	rm, err := Decode(fixturePath("richmessage_alltypes.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(rm.Blocks); got != 37 {
		t.Fatalf("blocks = %d, want 37", got)
	}
}

func TestLoadGolden(t *testing.T) {
	got, err := LoadGolden(fixturePath("richmessage_alltypes.golden.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "# All Types Demo\n") {
		t.Fatalf("unexpected golden: %q", got)
	}
}

func testMedia() UploadedMedia {
	return UploadedMedia{
		MainPhoto:     &tg.InputPhoto{ID: 101},
		CollagePhotoA: &tg.InputPhoto{ID: 102},
		CollagePhotoB: &tg.InputPhoto{ID: 103},
		Video:         &tg.InputDocument{ID: 201},
		Audio:         &tg.InputDocument{ID: 202},
	}
}

func photoID(p tg.InputPhotoClass) int64 {
	if ph, ok := p.(*tg.InputPhoto); ok {
		return ph.ID
	}
	return 0
}

func docID(d tg.InputDocumentClass) int64 {
	if doc, ok := d.(*tg.InputDocument); ok {
		return doc.ID
	}
	return 0
}

func collectMediaIDs(blocks []tg.PageBlockClass) (photos, videos, audios []int64, maps int) {
	var walk func(tg.PageBlockClass)
	walk = func(b tg.PageBlockClass) {
		switch v := b.(type) {
		case *tg.PageBlockPhoto:
			photos = append(photos, v.PhotoID)
		case *tg.PageBlockVideo:
			videos = append(videos, v.VideoID)
		case *tg.PageBlockAudio:
			audios = append(audios, v.AudioID)
		case *tg.PageBlockCollage:
			for _, it := range v.Items {
				walk(it)
			}
		case *tg.PageBlockSlideshow:
			for _, it := range v.Items {
				walk(it)
			}
		case *tg.PageBlockCover:
			walk(v.Cover)
		case *tg.PageBlockDetails:
			for _, nb := range v.Blocks {
				walk(nb)
			}
		case *tg.PageBlockMap:
			maps++
		case *tg.InputPageBlockMap:
			maps++
		}
	}
	for _, b := range blocks {
		walk(b)
	}
	return
}

func findInputMap(blocks []tg.PageBlockClass) *tg.InputPageBlockMap {
	var found *tg.InputPageBlockMap
	var walk func(tg.PageBlockClass)
	walk = func(b tg.PageBlockClass) {
		switch v := b.(type) {
		case *tg.InputPageBlockMap:
			found = v
		case *tg.PageBlockCollage:
			for _, it := range v.Items {
				walk(it)
			}
		case *tg.PageBlockSlideshow:
			for _, it := range v.Items {
				walk(it)
			}
		case *tg.PageBlockCover:
			walk(v.Cover)
		case *tg.PageBlockDetails:
			for _, nb := range v.Blocks {
				walk(nb)
			}
		}
	}
	for _, b := range blocks {
		walk(b)
	}
	return found
}

func encodeRich(t *testing.T, rm tg.RichMessage) []byte {
	t.Helper()
	var buf bin.Buffer
	if err := rm.Encode(&buf); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return append([]byte(nil), buf.Buf...)
}

func TestPrepareForSend(t *testing.T) {
	rm, err := Decode(fixturePath("richmessage_alltypes.bin"))
	if err != nil {
		t.Fatal(err)
	}
	before := encodeRich(t, rm)

	media := testMedia()
	prepared, err := PrepareForSend(rm, media)
	if err != nil {
		t.Fatal(err)
	}

	if got := len(prepared.Photos); got != 3 {
		t.Fatalf("Photos len = %d, want 3", got)
	}
	if photoID(prepared.Photos[0]) != 101 || photoID(prepared.Photos[1]) != 102 || photoID(prepared.Photos[2]) != 103 {
		t.Fatalf("Photos IDs = %d/%d/%d, want 101/102/103",
			photoID(prepared.Photos[0]), photoID(prepared.Photos[1]), photoID(prepared.Photos[2]))
	}
	if got := len(prepared.Documents); got != 2 {
		t.Fatalf("Documents len = %d, want 2", got)
	}
	if docID(prepared.Documents[0]) != 201 || docID(prepared.Documents[1]) != 202 {
		t.Fatalf("Documents IDs = %d/%d, want 201/202",
			docID(prepared.Documents[0]), docID(prepared.Documents[1]))
	}

	photos, videos, audios, maps := collectMediaIDs(prepared.Blocks)
	if len(photos) != 3 || photos[0] != 101 || photos[1] != 102 || photos[2] != 103 {
		t.Fatalf("block photo IDs = %v, want [101 102 103]", photos)
	}
	if len(videos) != 1 || videos[0] != 201 {
		t.Fatalf("block video IDs = %v, want [201]", videos)
	}
	if len(audios) != 1 || audios[0] != 202 {
		t.Fatalf("block audio IDs = %v, want [202]", audios)
	}
	if maps != 1 {
		t.Fatalf("maps = %d, want 1", maps)
	}

	im := findInputMap(prepared.Blocks)
	if im == nil {
		t.Fatal("expected InputPageBlockMap in prepared blocks")
	}
	geo, ok := im.Geo.(*tg.InputGeoPoint)
	if !ok {
		t.Fatalf("map geo type = %T, want *tg.InputGeoPoint", im.Geo)
	}
	// Original fixture values (from dump).
	const wantLat = 25.195948984760435
	const wantLong = 55.273411930558964
	if geo.Lat != wantLat || geo.Long != wantLong {
		t.Fatalf("map geo lat/long = %v/%v, want %v/%v", geo.Lat, geo.Long, wantLat, wantLong)
	}
	if im.Zoom != 15 || im.W != 800 || im.H != 400 {
		t.Fatalf("map zoom/w/h = %d/%d/%d, want 15/800/400", im.Zoom, im.W, im.H)
	}

	// Input must be unchanged.
	after := encodeRich(t, rm)
	if !bytes.Equal(before, after) {
		t.Fatal("PrepareForSend mutated input RichMessage")
	}

	// Separately decoded copy also must remain unchanged if we prepare a different decode.
	rm2, err := Decode(fixturePath("richmessage_alltypes.bin"))
	if err != nil {
		t.Fatal(err)
	}
	before2 := encodeRich(t, rm2)
	if _, err := PrepareForSend(rm2, media); err != nil {
		t.Fatal(err)
	}
	after2 := encodeRich(t, rm2)
	if !bytes.Equal(before2, after2) {
		t.Fatal("PrepareForSend mutated separately decoded RichMessage")
	}
}

func TestPrepareForSendErrors(t *testing.T) {
	base, err := Decode(fixturePath("richmessage_alltypes.bin"))
	if err != nil {
		t.Fatal(err)
	}
	media := testMedia()

	t.Run("nil media fields", func(t *testing.T) {
		cases := []struct {
			name  string
			media UploadedMedia
		}{
			{"nil main photo", UploadedMedia{CollagePhotoA: media.CollagePhotoA, CollagePhotoB: media.CollagePhotoB, Video: media.Video, Audio: media.Audio}},
			{"nil collage A", UploadedMedia{MainPhoto: media.MainPhoto, CollagePhotoB: media.CollagePhotoB, Video: media.Video, Audio: media.Audio}},
			{"nil collage B", UploadedMedia{MainPhoto: media.MainPhoto, CollagePhotoA: media.CollagePhotoA, Video: media.Video, Audio: media.Audio}},
			{"nil video", UploadedMedia{MainPhoto: media.MainPhoto, CollagePhotoA: media.CollagePhotoA, CollagePhotoB: media.CollagePhotoB, Audio: media.Audio}},
			{"nil audio", UploadedMedia{MainPhoto: media.MainPhoto, CollagePhotoA: media.CollagePhotoA, CollagePhotoB: media.CollagePhotoB, Video: media.Video}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := PrepareForSend(base, tc.media)
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), "nil") && !strings.Contains(err.Error(), "missing") {
					t.Fatalf("error should mention nil/missing media: %v", err)
				}
			})
		}
	})

	t.Run("absent collage", func(t *testing.T) {
		rm := cloneRich(t, base)
		rm.Blocks = filterBlocks(rm.Blocks, func(b tg.PageBlockClass) bool {
			_, ok := b.(*tg.PageBlockCollage)
			return !ok
		})
		_, err := PrepareForSend(rm, media)
		if err == nil {
			t.Fatal("expected error for absent collage")
		}
	})

	t.Run("extra top-level photo", func(t *testing.T) {
		rm := cloneRich(t, base)
		// Insert another top-level photo after the existing one.
		extra := &tg.PageBlockPhoto{PhotoID: 999}
		var out []tg.PageBlockClass
		inserted := false
		for _, b := range rm.Blocks {
			out = append(out, b)
			if _, ok := b.(*tg.PageBlockPhoto); ok && !inserted {
				out = append(out, extra)
				inserted = true
			}
		}
		rm.Blocks = out
		_, err := PrepareForSend(rm, media)
		if err == nil {
			t.Fatal("expected error for extra top-level photo")
		}
	})

	t.Run("absent map", func(t *testing.T) {
		rm := cloneRich(t, base)
		rm.Blocks = filterBlocks(rm.Blocks, func(b tg.PageBlockClass) bool {
			_, ok := b.(*tg.PageBlockMap)
			return !ok
		})
		_, err := PrepareForSend(rm, media)
		if err == nil {
			t.Fatal("expected error for absent map")
		}
	})
}

func cloneRich(t *testing.T, rm tg.RichMessage) tg.RichMessage {
	t.Helper()
	var buf bin.Buffer
	if err := rm.Encode(&buf); err != nil {
		t.Fatalf("encode: %v", err)
	}
	var out tg.RichMessage
	if err := out.Decode(&bin.Buffer{Buf: buf.Buf}); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func filterBlocks(blocks []tg.PageBlockClass, keep func(tg.PageBlockClass) bool) []tg.PageBlockClass {
	var out []tg.PageBlockClass
	for _, b := range blocks {
		if keep(b) {
			out = append(out, b)
		}
	}
	return out
}
