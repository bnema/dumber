package docmodel

// NextHeading returns the cursor for the count-th heading after from. Motions
// are strict: a heading at from is not considered a match. A count less than or
// equal to zero is treated as one. When levels are provided, only headings with
// one of those levels match.
func (d Document) NextHeading(from Cursor, count int, levels ...int) (Cursor, bool) {
	return d.headingMotion(from, count, 1, levels)
}

// PreviousHeading returns the cursor for the count-th heading before from.
// Motions are strict: a heading at from is not considered a match. A count less
// than or equal to zero is treated as one. When levels are provided, only
// headings with one of those levels match.
func (d Document) PreviousHeading(from Cursor, count int, levels ...int) (Cursor, bool) {
	return d.headingMotion(from, count, -1, levels)
}

func (d Document) headingMotion(from Cursor, count, direction int, levels []int) (Cursor, bool) {
	start, ok := d.indexByID[from.BlockID]
	if !ok {
		return Cursor{}, false
	}

	if count <= 0 {
		count = 1
	}

	remaining := count
	for i := start + direction; i >= 0 && i < len(d.blocks); i += direction {
		block := d.blocks[i]
		if !block.IsHeading() || !headingLevelMatches(block.HeadingLevel, levels) {
			continue
		}

		remaining--
		if remaining == 0 {
			return Cursor{BlockID: block.ID}, true
		}
	}

	return Cursor{}, false
}

func headingLevelMatches(level int, levels []int) bool {
	if len(levels) == 0 {
		return true
	}

	for _, allowed := range levels {
		if level == allowed {
			return true
		}
	}
	return false
}
