# markup testdata

`richmessage_alltypes.bin` — TL-encoded `tg.RichMessage` captured from
@richtextdemobot's "All Types" demo (Bot API 10.1 rich message).

Re-capture (after a gotd upgrade breaks decode):
1. Restore a throwaway `internal/richcap/main.go` that connects (profile
   `default`, `TGC_CONFIG_DIR=/path/to/.tgc`), fetches the demo message, and
   writes `m.RichMessage.Encode(&buf)` bytes to this file.
2. `go run ./internal/richcap && rm -rf internal/richcap`
3. `UPDATE_GOLDEN=1 go test ./internal/markup/ -run TestRenderRichMessageGolden`
4. Eyeball `richmessage_alltypes.golden.md`, commit both files.
