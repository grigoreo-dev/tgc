package setup

import "strings"

// Managed rc-block and completion-file markers.
// Markers match only as full lines (trailing \r ignored for comparison), never
// as mid-line substrings of user content (e.g. echo '# >>> tgc >>>').
const (
	BlockStart = "# >>> tgc >>>"
	BlockEnd   = "# <<< tgc <<<"
	FileMarker = "# managed by tgc self setup"
)

// managedRange is a half-open byte range [start, end) covering one managed
// block from the BlockStart line through the BlockEnd line (end is the offset
// just after the end marker's line text, before its trailing newline).
// complete is false for an orphan BlockStart with no matching BlockEnd (end
// then equals len(content)).
type managedRange struct {
	start    int
	end      int
	complete bool
}

// UpsertBlock ensures content contains exactly one managed block with the
// given body (block should include BlockStart/BlockEnd). All other content is
// preserved byte-for-byte. Returns the new content and whether it changed.
//
// Invariant: after UpsertBlock there is exactly one full-line managed block.
// If multiple complete managed ranges exist, all are removed and a single
// block is inserted at the first range's start position. An orphan BlockStart
// (no BlockEnd) is treated as a range from that line through EOF so Upsert
// heals to one clean block rather than appending a second.
//
// Newline asymmetry vs RemoveBlock: on replace, the suffix after the end
// marker is preserved byte-for-byte (including a following newline).
// RemoveBlock eats one trailing newline after a removed range so surrounding
// lines abut cleanly without an extra blank line.
func UpsertBlock(content, block string) (string, bool) {
	// Strip a single trailing newline so we control termination on append.
	block = strings.TrimSuffix(block, "\n")

	ranges := findManagedRanges(content)
	if len(ranges) == 0 {
		if content == "" {
			return block + "\n", true
		}
		if strings.HasSuffix(content, "\n") {
			return content + block + "\n", true
		}
		return content + "\n" + block + "\n", true
	}

	// Remove all managed ranges; write the new block only at the first range.
	var b strings.Builder
	b.Grow(len(content) + len(block))
	cursor := 0
	for i, r := range ranges {
		if r.start < cursor {
			if r.end > cursor {
				cursor = r.end
			}
			continue
		}
		b.WriteString(content[cursor:r.start])
		if i == 0 {
			b.WriteString(block)
			// Preserve suffix after the first range (byte-for-byte), including
			// any newline that followed the old BlockEnd line.
		}
		cursor = r.end
	}
	if cursor < len(content) {
		b.WriteString(content[cursor:])
	}
	newContent := b.String()
	if newContent == content {
		return content, false
	}
	return newContent, true
}

// RemoveBlock removes every complete managed tgc block (full-line markers).
// Orphan BlockStart without BlockEnd is not a complete range and is left
// untouched. Non-block content is preserved.
//
// Newline asymmetry vs UpsertBlock: after removing a range, one trailing
// newline immediately following the BlockEnd line is eaten so surrounding
// lines abut without an extra blank. UpsertBlock replace preserves that
// suffix newline instead.
func RemoveBlock(content string) (string, bool) {
	ranges := findManagedRanges(content)
	var complete []managedRange
	for _, r := range ranges {
		if r.complete {
			complete = append(complete, r)
		}
	}
	if len(complete) == 0 {
		return content, false
	}

	var b strings.Builder
	b.Grow(len(content))
	cursor := 0
	for _, r := range complete {
		if r.start < cursor {
			continue
		}
		b.WriteString(content[cursor:r.start])
		cursor = r.end
		if cursor < len(content) && content[cursor] == '\n' {
			cursor++
		}
	}
	if cursor < len(content) {
		b.WriteString(content[cursor:])
	}
	return b.String(), true
}

// findManagedRanges returns managed block ranges in content order.
// Markers match only as full lines. A complete range runs from a BlockStart
// line through the next BlockEnd line. An orphan BlockStart (no subsequent
// BlockEnd) yields a range from that start to EOF with complete=false.
// Orphan BlockEnd lines alone are ignored (user content).
func findManagedRanges(content string) []managedRange {
	type lineRef struct {
		lo, hi int    // [lo, hi) of line text excluding trailing \n
		text   string // line without \n; trailing \r stripped for compare
	}
	var lines []lineRef
	for i := 0; i < len(content); {
		j := i
		for j < len(content) && content[j] != '\n' {
			j++
		}
		text := content[i:j]
		cmp := strings.TrimSuffix(text, "\r")
		lines = append(lines, lineRef{lo: i, hi: j, text: cmp})
		if j < len(content) && content[j] == '\n' {
			i = j + 1
		} else {
			break
		}
	}

	var ranges []managedRange
	for li := 0; li < len(lines); li++ {
		if lines[li].text != BlockStart {
			continue
		}
		start := lines[li].lo
		found := -1
		for lj := li + 1; lj < len(lines); lj++ {
			if lines[lj].text == BlockEnd {
				found = lj
				break
			}
		}
		if found >= 0 {
			ranges = append(ranges, managedRange{
				start:    start,
				end:      lines[found].hi,
				complete: true,
			})
			li = found
			continue
		}
		// Orphan start → span through EOF for Upsert heal.
		ranges = append(ranges, managedRange{
			start:    start,
			end:      len(content),
			complete: false,
		})
		break
	}
	return ranges
}
