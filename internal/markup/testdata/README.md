# markup testdata

Canonical All Types rich fixture (Bot API 10.1), captured from
@richtextdemobot's "All Types" demo:

| File | Role |
|------|------|
| `richmessage_alltypes.bin` | TL-encoded `tg.RichMessage` (37 blocks) |
| `richmessage_alltypes.golden.md` | Byte-exact Markdown projection for unit tests and the full-live gate |

These files are **maintained repository data**. Unit coverage is
`TestRenderRichMessageGolden` plus `internal/richfixture` structural tests
(37 blocks, expected media shape). Live release verification is
`scripts/e2e/08-rich-all-types.sh` (not default `run-all.sh`).

## When to recapture

Re-capture only when:

- a **gotd upgrade** breaks decode of the committed `.bin` (decoder error text
  points at this upgrade case), or
- Telegram / @richtextdemobot changes the All Types demo in a way you
  intentionally want to adopt.

Do **not** treat recapture as a throwaway one-liner that skips review.

## Recapture procedure (maintained path)

1. Capture a fresh TL-encoded `RichMessage` for the All Types demo into
   `internal/markup/testdata/richmessage_alltypes.bin` using a **local,
   uncommitted** helper (e.g. under `/tmp` or a disposable path). Do **not**
   add exploratory trees such as `internal/richcap`, `internal/richjson`,
   `internal/richmd`, or `internal/richbuild` to git.
2. Decode must succeed with the current gotd/tg types. Prefer the maintained
   package entrypoints (`internal/richfixture.Decode` / loaders used by tests)
   rather than a second ad-hoc decoder.
3. Regenerate the golden:

   ```sh
   UPDATE_GOLDEN=1 go test ./internal/markup/ -run TestRenderRichMessageGolden
   ```

4. **Before accepting**, review **both**:
   - the **37-block structural** fixture tests (block count, media shape,
     prepare-for-send invariants):

     ```sh
     go test ./internal/richfixture/ -count=1
     ```

   - the **golden diff** for `richmessage_alltypes.golden.md` (full Markdown
     projection; no silent acceptance of large or unexpected drift).
5. Commit only the reviewed `.bin` + `.golden.md` pair (and any intentional
   test updates). Re-run full-live when media or fixture shape changed (see
   `scripts/e2e/README.md` for the exact setup + gate procedure, including
   required `setup.sh` / `.env.generated` for live runs):

   ```sh
   bash scripts/e2e/setup.sh
   bash scripts/e2e/08-rich-all-types.sh
   ```

## Notes

- Golden compare is byte-exact (`jq -j '.text'` + `cmp` on live; unit golden
  via `richfixture.LoadGolden`).
- Full-live leaves the verified recipient-side message visible by default;
  set `E2E_RICH_CLEANUP=1` to delete after success (see e2e README).
- Required live media basenames under `E2E_RICH_MEDIA_DIR` (default
  `/tmp/demo_media`): `photo_0.jpg`, `photo_1.jpg`, `photo_2.jpg`,
  `dubaiVideo.mp4`, `Neon Rain Train.mp3`.
