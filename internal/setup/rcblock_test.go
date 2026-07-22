package setup

import (
	"strings"
	"testing"
)

func sampleBlock(body string) string {
	return BlockStart + "\n" + body + "\n" + BlockEnd
}

func TestUpsertBlock_InsertEmpty(t *testing.T) {
	block := sampleBlock(`export PATH="/opt/tgc/bin:$PATH"`)
	got, changed := UpsertBlock("", block)
	if !changed {
		t.Fatal("expected changed=true when inserting into empty content")
	}
	want := block + "\n"
	if got != want {
		t.Fatalf("UpsertBlock empty:\ngot:\n%q\nwant:\n%q", got, want)
	}
	if strings.Count(got, BlockStart) != 1 || strings.Count(got, BlockEnd) != 1 {
		t.Fatalf("expected exactly one block, got:\n%s", got)
	}
}

func TestUpsertBlock_AppendWithSeparatingNewline(t *testing.T) {
	// Content without trailing newline: must add separating newline before block.
	content := "export FOO=1"
	block := sampleBlock(`export PATH="/opt/tgc/bin:$PATH"`)
	got, changed := UpsertBlock(content, block)
	if !changed {
		t.Fatal("expected changed=true on append")
	}
	// Surrounding content preserved byte-for-byte; only separator + block added.
	if !strings.HasPrefix(got, content) {
		t.Fatalf("prefix not preserved: got %q", got)
	}
	// After original content there must be exactly one newline then the block.
	rest := got[len(content):]
	if !strings.HasPrefix(rest, "\n"+block) {
		t.Fatalf("expected separating newline then block; rest=%q", rest)
	}
	if countFullLine(got, BlockStart) != 1 {
		t.Fatalf("want one full-line BlockStart, got:\n%s", got)
	}
}

func TestUpsertBlock_AppendWhenTrailingNewlinePresent(t *testing.T) {
	content := "export FOO=1\n"
	block := sampleBlock(`export PATH="/opt/tgc/bin:$PATH"`)
	got, changed := UpsertBlock(content, block)
	if !changed {
		t.Fatal("expected changed=true on append")
	}
	want := content + block + "\n"
	if got != want {
		t.Fatalf("append with trailing newline:\ngot:\n%q\nwant:\n%q", got, want)
	}
}

func TestUpsertBlock_UpdateInPlace(t *testing.T) {
	prefix := "# user config\nexport EDITOR=vim\n"
	oldBlock := sampleBlock(`export PATH="/old/bin:$PATH"`)
	newBlock := sampleBlock(`export PATH="/new/bin:$PATH"`)
	// content layout: prefix + oldBlock + "\n" + "# after block\n..."
	content := prefix + oldBlock + "\n" + "# after block\nalias ll='ls -l'\n"

	got, changed := UpsertBlock(content, newBlock)
	if !changed {
		t.Fatal("expected changed=true when updating stale block")
	}
	if !strings.HasPrefix(got, prefix) {
		t.Fatalf("prefix not preserved byte-for-byte:\ngot prefix %q\nwant %q", got[:len(prefix)], prefix)
	}
	// Find block region and ensure surroundings match.
	start := strings.Index(got, BlockStart)
	end := strings.Index(got, BlockEnd)
	if start < 0 || end < 0 {
		t.Fatalf("block markers missing in result:\n%s", got)
	}
	// Prefix before BlockStart must equal original prefix.
	if got[:start] != prefix {
		t.Fatalf("content before block changed:\ngot %q\nwant %q", got[:start], prefix)
	}
	// After BlockEnd should preserve the trailing section that followed the old block.
	afterOld := content[strings.Index(content, BlockEnd)+len(BlockEnd):]
	afterNew := got[end+len(BlockEnd):]
	if afterNew != afterOld {
		t.Fatalf("content after block not preserved:\ngot %q\nwant %q", afterNew, afterOld)
	}
	if !strings.Contains(got, `/new/bin`) {
		t.Fatalf("new block body missing: %s", got)
	}
	if strings.Contains(got, `/old/bin`) {
		t.Fatalf("old block body still present: %s", got)
	}
	if strings.Count(got, BlockStart) != 1 {
		t.Fatalf("expected exactly one block: %s", got)
	}
}

func TestUpsertBlock_Idempotent(t *testing.T) {
	content := "# keep me\n"
	block := sampleBlock(`export PATH="/opt/tgc/bin:$PATH"`)
	once, changed1 := UpsertBlock(content, block)
	if !changed1 {
		t.Fatal("first upsert should change")
	}
	twice, changed2 := UpsertBlock(once, block)
	if changed2 {
		t.Fatal("second upsert should report changed=false")
	}
	if twice != once {
		t.Fatalf("idempotent mismatch:\nfirst:\n%q\nsecond:\n%q", once, twice)
	}
}

func TestRemoveBlock_Present(t *testing.T) {
	prefix := "before\n"
	suffix := "after\n"
	block := sampleBlock(`export PATH="/opt/tgc/bin:$PATH"`)
	content := prefix + block + "\n" + suffix

	got, changed := RemoveBlock(content)
	if !changed {
		t.Fatal("expected changed=true when block present")
	}
	if strings.Contains(got, BlockStart) || strings.Contains(got, BlockEnd) {
		t.Fatalf("block markers still present:\n%s", got)
	}
	if strings.Contains(got, "/opt/tgc/bin") {
		t.Fatalf("block body still present:\n%s", got)
	}
	// Only the marked block removed; surrounding content retained.
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Fatalf("surrounding content lost: %q", got)
	}
}

func TestRemoveBlock_Absent(t *testing.T) {
	content := "export FOO=1\n# not our block\n"
	got, changed := RemoveBlock(content)
	if changed {
		t.Fatal("expected changed=false when no block")
	}
	if got != content {
		t.Fatalf("content mutated without block:\ngot %q\nwant %q", got, content)
	}
}

func TestRemoveBlock_OnlyBlockRemoved(t *testing.T) {
	// Ensure other similar-looking comments survive.
	content := strings.Join([]string{
		"# >>> other tool >>>",
		"keep this",
		"# <<< other tool <<<",
		BlockStart,
		`export PATH="/opt/tgc/bin:$PATH"`,
		BlockEnd,
		"tail",
		"",
	}, "\n")

	got, changed := RemoveBlock(content)
	if !changed {
		t.Fatal("expected change")
	}
	if strings.Contains(got, BlockStart) || strings.Contains(got, `export PATH="/opt/tgc/bin:$PATH"`) {
		t.Fatalf("tgc block not fully removed:\n%s", got)
	}
	if !strings.Contains(got, "# >>> other tool >>>") || !strings.Contains(got, "keep this") {
		t.Fatalf("unrelated content removed:\n%s", got)
	}
	if !strings.Contains(got, "tail") {
		t.Fatalf("tail lost:\n%s", got)
	}
}

func TestConstants(t *testing.T) {
	if BlockStart != "# >>> tgc >>>" {
		t.Fatalf("BlockStart = %q", BlockStart)
	}
	if BlockEnd != "# <<< tgc <<<" {
		t.Fatalf("BlockEnd = %q", BlockEnd)
	}
	if FileMarker != "# managed by tgc self setup" {
		t.Fatalf("FileMarker = %q", FileMarker)
	}
}

// C1: multiple managed blocks collapse to exactly one at the first range's position.
func TestUpsertBlock_CollapseMultipleBlocks(t *testing.T) {
	prefix := "export EDITOR=vim\n"
	mid := "alias ll='ls -l'\n"
	suffix := "export LANG=C\n"
	old1 := sampleBlock(`export PATH="/old1:$PATH"`)
	old2 := sampleBlock(`export PATH="/old2:$PATH"`)
	content := prefix + old1 + "\n" + mid + old2 + "\n" + suffix

	newBlock := sampleBlock(`export PATH="/new:$PATH"`)
	got, changed := UpsertBlock(content, newBlock)
	if !changed {
		t.Fatal("expected change when collapsing multiple blocks")
	}
	if strings.Count(got, BlockStart) != 1 || strings.Count(got, BlockEnd) != 1 {
		t.Fatalf("want exactly one managed block, got:\n%s", got)
	}
	if strings.Contains(got, "/old1") || strings.Contains(got, "/old2") {
		t.Fatalf("stale block bodies remain:\n%s", got)
	}
	if !strings.Contains(got, "/new") {
		t.Fatalf("new block body missing:\n%s", got)
	}
	// Inserted at first range position: prefix must precede the single block.
	idx := strings.Index(got, BlockStart)
	if idx < 0 {
		t.Fatalf("block missing after collapse:\n%s", got)
	}
	if got[:idx] != prefix {
		t.Fatalf("block not placed at first range position:\nprefix got %q\nwant %q\nfull:\n%s",
			got[:idx], prefix, got)
	}
	// Non-block content preserved.
	if !strings.Contains(got, "alias ll='ls -l'") || !strings.Contains(got, "export LANG=C") {
		t.Fatalf("surrounding non-block content lost:\n%s", got)
	}
	// Second apply is idempotent.
	again, changed2 := UpsertBlock(got, newBlock)
	if changed2 || again != got {
		t.Fatalf("collapse result not idempotent:\nfirst %q\nsecond %q changed=%v", got, again, changed2)
	}
}

// I1: mid-line / substring marker text must never act as block boundaries.
func TestUpsertBlock_SubstringMarkersUntouched(t *testing.T) {
	// User lines that merely mention the marker text.
	content := strings.Join([]string{
		`echo '# >>> tgc >>>'`,
		`# note: see docs for # >>> tgc >>> style markers`,
		`echo '# <<< tgc <<<'`,
		"",
	}, "\n")
	block := sampleBlock(`export PATH="/opt/tgc/bin:$PATH"`)

	got, changed := UpsertBlock(content, block)
	if !changed {
		t.Fatal("expected append when no full-line managed block exists")
	}
	// Original user lines must survive byte-for-byte as a prefix (append path).
	if !strings.HasPrefix(got, content) {
		// content may gain only a block after it; if content lacks trailing NL
		// we add one — but our content ends with "\n" so prefix must hold.
		t.Fatalf("user lines mutated:\ngot:\n%q\nwant prefix:\n%q", got, content)
	}
	if countFullLine(got, BlockStart) != 1 || countFullLine(got, BlockEnd) != 1 {
		t.Fatalf("want one full-line start/end, got starts=%d ends=%d\n%s",
			countFullLine(got, BlockStart), countFullLine(got, BlockEnd), got)
	}
	// Echo lines still present unchanged.
	if !strings.Contains(got, `echo '# >>> tgc >>>'`) || !strings.Contains(got, `echo '# <<< tgc <<<'`) {
		t.Fatalf("substring marker lines rewritten:\n%s", got)
	}
}

func TestRemoveBlock_SubstringMarkersUntouched(t *testing.T) {
	content := strings.Join([]string{
		`echo '# >>> tgc >>>'`,
		`echo '# <<< tgc <<<'`,
		"keep me",
		"",
	}, "\n")
	got, changed := RemoveBlock(content)
	if changed {
		t.Fatal("expected changed=false when only substring markers present")
	}
	if got != content {
		t.Fatalf("substring-only content mutated:\ngot %q\nwant %q", got, content)
	}
}

// I2: orphan BlockStart without BlockEnd heals to one clean block (no append of a second).
// Orphan spans from the start line through EOF (typical incomplete write at file end).
func TestUpsertBlock_OrphanStartHeals(t *testing.T) {
	prefix := "export FOO=1\nexport BAR=2\n"
	orphan := BlockStart + "\n" + `export PATH="/orphan:$PATH"` + "\n# no end marker\n"
	content := prefix + orphan
	newBlock := sampleBlock(`export PATH="/healed:$PATH"`)

	got, changed := UpsertBlock(content, newBlock)
	if !changed {
		t.Fatal("expected change when healing orphan start")
	}
	if countFullLine(got, BlockStart) != 1 || countFullLine(got, BlockEnd) != 1 {
		t.Fatalf("want exactly one clean block after heal:\n%s", got)
	}
	if strings.Contains(got, "/orphan") {
		t.Fatalf("orphan body still present:\n%s", got)
	}
	if !strings.Contains(got, "/healed") {
		t.Fatalf("healed block missing:\n%s", got)
	}
	// Placed at orphan start position; prior user content preserved.
	if !strings.HasPrefix(got, prefix) {
		t.Fatalf("prefix not preserved: %q", got)
	}
	// Must not have appended a second block after the orphan material.
	if strings.Count(got, "/healed") != 1 {
		t.Fatalf("duplicate healed body:\n%s", got)
	}
}

func TestUpsertBlock_OrphanEndIgnoredThenAppend(t *testing.T) {
	// Lone full-line BlockEnd is not a managed range; treat as user content and append.
	content := "export FOO=1\n" + BlockEnd + "\nexport BAR=2\n"
	newBlock := sampleBlock(`export PATH="/new:$PATH"`)
	got, changed := UpsertBlock(content, newBlock)
	if !changed {
		t.Fatal("expected change")
	}
	if !strings.HasPrefix(got, content) {
		t.Fatalf("original content not preserved as prefix:\ngot:\n%q\nwant prefix:\n%q", got, content)
	}
	if countFullLine(got, BlockStart) != 1 {
		t.Fatalf("want one BlockStart after append, got %d:\n%s", countFullLine(got, BlockStart), got)
	}
	// User orphan end line + the new block's end → two full-line BlockEnd markers.
	if countFullLine(got, BlockEnd) != 2 {
		t.Fatalf("want two BlockEnd lines (orphan user line + new block), got %d:\n%s",
			countFullLine(got, BlockEnd), got)
	}
	if !strings.Contains(got, "/new") {
		t.Fatalf("new block missing:\n%s", got)
	}
}

func TestRemoveBlock_OrphanStartNoOp(t *testing.T) {
	// Orphan start without end is not a complete managed range → no-op.
	content := "before\n" + BlockStart + "\norphan body\n" + "after\n"
	got, changed := RemoveBlock(content)
	if changed {
		t.Fatal("expected changed=false for incomplete orphan start")
	}
	if got != content {
		t.Fatalf("orphan start content mutated:\ngot %q\nwant %q", got, content)
	}
}

// I3: exact post-RemoveBlock byte equality for a well-formed mid-file block.
func TestRemoveBlock_ExactBytes(t *testing.T) {
	content := "before\n" + sampleBlock(`export PATH="/opt/tgc/bin:$PATH"`) + "\nafter\n"
	got, changed := RemoveBlock(content)
	if !changed {
		t.Fatal("expected change")
	}
	want := "before\nafter\n"
	if got != want {
		t.Fatalf("exact bytes mismatch:\ngot  %q\nwant %q", got, want)
	}
}

func countFullLine(content, line string) int {
	n := 0
	start := 0
	for i := 0; i <= len(content); i++ {
		if i == len(content) || content[i] == '\n' {
			if content[start:i] == line {
				n++
			}
			start = i + 1
		}
	}
	return n
}
