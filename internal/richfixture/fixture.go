// Package richfixture loads and prepares the captured All Types rich-message
// fixture for unit tests and send-path demos.
package richfixture

import (
	"fmt"
	"os"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

// Decode reads a TL-encoded tg.RichMessage from path.
func Decode(path string) (tg.RichMessage, error) {
	raw, err := os.ReadFile(path) //#nosec G304 -- path is a caller-supplied fixture/golden path for tests and demos, not untrusted input
	if err != nil {
		return tg.RichMessage{}, fmt.Errorf("read fixture %s: %w", path, err)
	}
	var rm tg.RichMessage
	if err := rm.Decode(&bin.Buffer{Buf: raw}); err != nil {
		return tg.RichMessage{}, fmt.Errorf(
			"fixture decode failed — gotd version may have changed; "+
				"re-capture with internal/markup/testdata/README.md instructions: %w", err)
	}
	return rm, nil
}

// LoadGolden reads a golden Markdown file, preserving exact bytes as a string.
func LoadGolden(path string) (string, error) {
	raw, err := os.ReadFile(path) //#nosec G304 -- path is a caller-supplied fixture/golden path for tests and demos, not untrusted input
	if err != nil {
		return "", fmt.Errorf("read golden %s: %w", path, err)
	}
	return string(raw), nil
}

// UploadedMedia holds caller-supplied input media that replace fixture media IDs.
type UploadedMedia struct {
	MainPhoto     tg.InputPhotoClass
	CollagePhotoA tg.InputPhotoClass
	CollagePhotoB tg.InputPhotoClass
	Video         tg.InputDocumentClass
	Audio         tg.InputDocumentClass
}

// PrepareForSend deep-copies rm, remaps media references from tree structure
// onto the supplied uploads, and converts PageBlockMap to InputPageBlockMap.
func PrepareForSend(rm tg.RichMessage, media UploadedMedia) (*tg.InputRichMessage, error) {
	if err := validateMedia(media); err != nil {
		return nil, err
	}

	cloned, err := deepCopyRich(rm)
	if err != nil {
		return nil, err
	}

	shape, err := discoverShape(cloned.Blocks)
	if err != nil {
		return nil, err
	}

	if err := remapBlocks(cloned.Blocks, shape, media); err != nil {
		return nil, err
	}

	return &tg.InputRichMessage{
		Rtl:       cloned.Rtl,
		Blocks:    cloned.Blocks,
		Photos:    []tg.InputPhotoClass{media.MainPhoto, media.CollagePhotoA, media.CollagePhotoB},
		Documents: []tg.InputDocumentClass{media.Video, media.Audio},
	}, nil
}

func validateMedia(media UploadedMedia) error {
	switch {
	case media.MainPhoto == nil:
		return fmt.Errorf("missing uploaded media: nil main photo")
	case media.CollagePhotoA == nil:
		return fmt.Errorf("missing uploaded media: nil collage photo A")
	case media.CollagePhotoB == nil:
		return fmt.Errorf("missing uploaded media: nil collage photo B")
	case media.Video == nil:
		return fmt.Errorf("missing uploaded media: nil video")
	case media.Audio == nil:
		return fmt.Errorf("missing uploaded media: nil audio")
	}
	return nil
}

func deepCopyRich(rm tg.RichMessage) (tg.RichMessage, error) {
	var buf bin.Buffer
	if err := rm.Encode(&buf); err != nil {
		return tg.RichMessage{}, fmt.Errorf("clone encode: %w", err)
	}
	var out tg.RichMessage
	if err := out.Decode(&bin.Buffer{Buf: buf.Buf}); err != nil {
		return tg.RichMessage{}, fmt.Errorf("clone decode: %w", err)
	}
	return out, nil
}

// fixtureShape holds media IDs discovered from the block tree (not from Photos/Documents arrays).
type fixtureShape struct {
	mainPhotoID     int64
	collagePhotoIDs [2]int64
	videoID         int64
	audioID         int64
	mapIndex        int // index in top-level Blocks
}

func discoverShape(blocks []tg.PageBlockClass) (fixtureShape, error) {
	var s fixtureShape
	var mainPhotos, videos, audios, collages, maps int
	s.mapIndex = -1

	for i, b := range blocks {
		switch v := b.(type) {
		case *tg.PageBlockPhoto:
			mainPhotos++
			if mainPhotos == 1 {
				s.mainPhotoID = v.PhotoID
			}
		case *tg.PageBlockVideo:
			videos++
			if videos == 1 {
				s.videoID = v.VideoID
			}
		case *tg.PageBlockAudio:
			audios++
			if audios == 1 {
				s.audioID = v.AudioID
			}
		case *tg.PageBlockCollage:
			collages++
			if collages == 1 {
				ids, err := collagePhotoIDs(v)
				if err != nil {
					return s, err
				}
				s.collagePhotoIDs = ids
			}
		case *tg.PageBlockMap:
			maps++
			if maps == 1 {
				s.mapIndex = i
			}
		}
	}

	switch {
	case mainPhotos == 0:
		return s, fmt.Errorf("malformed fixture: absent top-level photo")
	case mainPhotos > 1:
		return s, fmt.Errorf("malformed fixture: expected exactly one top-level photo, found %d", mainPhotos)
	case videos == 0:
		return s, fmt.Errorf("malformed fixture: absent top-level video")
	case videos > 1:
		return s, fmt.Errorf("malformed fixture: expected exactly one top-level video, found %d", videos)
	case audios == 0:
		return s, fmt.Errorf("malformed fixture: absent top-level audio")
	case audios > 1:
		return s, fmt.Errorf("malformed fixture: expected exactly one top-level audio, found %d", audios)
	case collages == 0:
		return s, fmt.Errorf("malformed fixture: absent collage")
	case collages > 1:
		return s, fmt.Errorf("malformed fixture: expected exactly one collage, found %d", collages)
	case maps == 0:
		return s, fmt.Errorf("malformed fixture: absent map")
	case maps > 1:
		return s, fmt.Errorf("malformed fixture: expected exactly one map, found %d", maps)
	}
	return s, nil
}

func collagePhotoIDs(c *tg.PageBlockCollage) ([2]int64, error) {
	var ids [2]int64
	if len(c.Items) != 2 {
		return ids, fmt.Errorf("malformed fixture: collage must have exactly 2 photo items, found %d", len(c.Items))
	}
	for i, it := range c.Items {
		ph, ok := it.(*tg.PageBlockPhoto)
		if !ok {
			return ids, fmt.Errorf("malformed fixture: collage item %d is %T, want *tg.PageBlockPhoto", i, it)
		}
		ids[i] = ph.PhotoID
	}
	return ids, nil
}

func remapBlocks(blocks []tg.PageBlockClass, shape fixtureShape, media UploadedMedia) error {
	mainID := inputPhotoID(media.MainPhoto)
	colA := inputPhotoID(media.CollagePhotoA)
	colB := inputPhotoID(media.CollagePhotoB)
	videoID := inputDocID(media.Video)
	audioID := inputDocID(media.Audio)

	for i, b := range blocks {
		switch v := b.(type) {
		case *tg.PageBlockPhoto:
			if v.PhotoID == shape.mainPhotoID {
				v.PhotoID = mainID
			}
		case *tg.PageBlockVideo:
			if v.VideoID == shape.videoID {
				v.VideoID = videoID
			}
		case *tg.PageBlockAudio:
			if v.AudioID == shape.audioID {
				v.AudioID = audioID
			}
		case *tg.PageBlockCollage:
			for _, it := range v.Items {
				if ph, ok := it.(*tg.PageBlockPhoto); ok {
					switch ph.PhotoID {
					case shape.collagePhotoIDs[0]:
						ph.PhotoID = colA
					case shape.collagePhotoIDs[1]:
						ph.PhotoID = colB
					}
				}
			}
		case *tg.PageBlockMap:
			if i == shape.mapIndex {
				converted, err := convertMap(v)
				if err != nil {
					return err
				}
				blocks[i] = converted
			}
		}
	}
	return nil
}

func convertMap(m *tg.PageBlockMap) (tg.PageBlockClass, error) {
	geo, ok := m.Geo.(*tg.GeoPoint)
	if !ok {
		return nil, fmt.Errorf("malformed fixture: map geo is %T, want *tg.GeoPoint", m.Geo)
	}
	return &tg.InputPageBlockMap{
		Zoom:    m.Zoom,
		W:       m.W,
		H:       m.H,
		Caption: m.Caption,
		Geo: &tg.InputGeoPoint{
			Lat:  geo.Lat,
			Long: geo.Long,
		},
	}, nil
}

func inputPhotoID(p tg.InputPhotoClass) int64 {
	if ph, ok := p.(*tg.InputPhoto); ok {
		return ph.ID
	}
	// Other InputPhotoClass variants (e.g. InputPhotoEmpty) have no usable ID.
	return 0
}

func inputDocID(d tg.InputDocumentClass) int64 {
	if doc, ok := d.(*tg.InputDocument); ok {
		return doc.ID
	}
	return 0
}
