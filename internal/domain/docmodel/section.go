package docmodel

// SectionRange returns the half-open range for the section containing from.
// The range starts at the containing heading and ends before the first
// subsequent heading whose level is less than or equal to the start heading's
// level. A zero EndExclusive cursor represents the end of the document.
func (d Document) SectionRange(from Cursor) (BlockRange, bool) {
	fromIndex, ok := d.indexByID[from.BlockID]
	if !ok {
		return BlockRange{}, false
	}

	startIndex, ok := d.containingHeadingIndex(fromIndex)
	if !ok {
		return BlockRange{}, false
	}

	start := d.blocks[startIndex]
	section := BlockRange{
		StartInclusive: Cursor{BlockID: start.ID},
	}

	for i := startIndex + 1; i < len(d.blocks); i++ {
		block := d.blocks[i]
		if block.IsHeading() && block.HeadingLevel <= start.HeadingLevel {
			section.EndExclusive = Cursor{BlockID: block.ID}
			return section, true
		}
	}

	return section, true
}

func (d Document) containingHeadingIndex(fromIndex int) (int, bool) {
	for i := fromIndex; i >= 0; i-- {
		if d.blocks[i].IsHeading() {
			return i, true
		}
	}
	return 0, false
}
