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
	// Ensure no double-blank mangling of original line.
	if strings.HasPrefix(got, content+"\n\n") {
		// allowed only if block itself starts with newline (it doesn't)
	}
	if strings.Count(got, BlockStart) != 1 {
		t.Fatalf("want one BlockStart, got:\n%s", got)
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
	suffix := "\n# after block\nalias ll='ls -l'\n"
	oldBlock := sampleBlock(`export PATH="/old/bin:$PATH"`)
	newBlock := sampleBlock(`export PATH="/new/bin:$PATH"`)
	content := prefix + oldBlock + "\n" + strings.TrimPrefix(suffix, "\n")
	// content layout: prefix + oldBlock + "\n" + "# after block\n..."
	// Rebuild carefully for byte-stable prefix/suffix.
	content = prefix + oldBlock + "\n" + "# after block\nalias ll='ls -l'\n"

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
