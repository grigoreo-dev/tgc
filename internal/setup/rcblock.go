package setup

import "strings"

// Managed rc-block and completion-file markers.
const (
	BlockStart = "# >>> tgc >>>"
	BlockEnd   = "# <<< tgc <<<"
	FileMarker = "# managed by tgc self setup"
)

// UpsertBlock ensures content contains exactly one managed block with the
// given body (block should include BlockStart/BlockEnd). All other content is
// preserved byte-for-byte. Returns the new content and whether it changed.
func UpsertBlock(content, block string) (string, bool) {
	// Normalize block: strip a single trailing newline so we control termination.
	block = strings.TrimSuffix(block, "\n")

	start := strings.Index(content, BlockStart)
	end := strings.Index(content, BlockEnd)

	if start >= 0 && end > start {
		// Replace existing block in place (from BlockStart through BlockEnd).
		endExclusive := end + len(BlockEnd)
		// If a single newline immediately follows the end marker, include it in
		// the replaced span so we don't leave a blank line when the new block
		// is written with its own trailing newline.
		prefix := content[:start]
		suffix := content[endExclusive:]
		// Drop one leading newline from suffix if present — we re-add structure
		// via the block's trailing newline + residual suffix content.
		// Actually: preserve suffix byte-for-byte after the end marker. The
		// block region is [start, endExclusive). What follows stays as-is.
		newContent := prefix + block + suffix
		// Ensure file still ends reasonably: if old content after end had no
		// newline and we need nothing else — leave as-is.
		if newContent == content {
			return content, false
		}
		return newContent, true
	}

	// No existing block: append with a separating newline when needed.
	if content == "" {
		return block + "\n", true
	}
	if strings.HasSuffix(content, "\n") {
		return content + block + "\n", true
	}
	return content + "\n" + block + "\n", true
}

// RemoveBlock removes the managed tgc block if present. Returns the new content
// and whether a block was removed. Non-block content is preserved.
func RemoveBlock(content string) (string, bool) {
	start := strings.Index(content, BlockStart)
	end := strings.Index(content, BlockEnd)
	if start < 0 || end < start {
		return content, false
	}
	endExclusive := end + len(BlockEnd)
	prefix := content[:start]
	suffix := content[endExclusive:]
	// If the block was followed by a newline, drop one so we don't leave a
	// double blank where the block was (optional cleanup). Prefer preserving
	// suffix bytes when the newline is part of "\n" + rest.
	// When block is "\n"-terminated in the file (common: "...BlockEnd\n..."),
	// consume that single newline with the block so surrounding lines abut
	// cleanly without an extra blank line introduced by the marker lines.
	if strings.HasPrefix(suffix, "\n") {
		suffix = suffix[1:]
	}
	// If prefix ends with a newline and we removed a mid-file block, keep
	// prefix as-is (it already separates previous content).
	return prefix + suffix, true
}
