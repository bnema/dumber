package docmodel

import (
	"reflect"
	"testing"
)

func TestSectionRange(t *testing.T) {
	doc := sectionRangeDocument(t)

	tests := []struct {
		name string
		from Cursor
		want BlockRange
		ok   bool
	}{
		{
			name: "top level section includes descendant headings and stops before next top level heading",
			from: Cursor{BlockID: "h1"},
			want: BlockRange{
				StartInclusive: Cursor{BlockID: "h1"},
				EndExclusive:   Cursor{BlockID: "h1b"},
			},
			ok: true,
		},
		{
			name: "non-heading anchor resolves to nearest preceding heading",
			from: Cursor{BlockID: "p-h2"},
			want: BlockRange{
				StartInclusive: Cursor{BlockID: "h2"},
				EndExclusive:   Cursor{BlockID: "h2b"},
			},
			ok: true,
		},
		{
			name: "non-heading anchor resolves to innermost preceding heading",
			from: Cursor{BlockID: "p-h3"},
			want: BlockRange{
				StartInclusive: Cursor{BlockID: "h3"},
				EndExclusive:   Cursor{BlockID: "h3b"},
			},
			ok: true,
		},
		{
			name: "nested section includes deeper descendant heading",
			from: Cursor{BlockID: "h2"},
			want: BlockRange{
				StartInclusive: Cursor{BlockID: "h2"},
				EndExclusive:   Cursor{BlockID: "h2b"},
			},
			ok: true,
		},
		{
			name: "descendant heading section ends before next heading at same level",
			from: Cursor{BlockID: "h3"},
			want: BlockRange{
				StartInclusive: Cursor{BlockID: "h3"},
				EndExclusive:   Cursor{BlockID: "h3b"},
			},
			ok: true,
		},
		{
			name: "skipped heading levels are valid descendants",
			from: Cursor{BlockID: "h2-skip"},
			want: BlockRange{
				StartInclusive: Cursor{BlockID: "h2-skip"},
				EndExclusive:   Cursor{BlockID: "h2-sibling"},
			},
			ok: true,
		},
		{
			name: "adjacent same-level heading yields empty body range ending at adjacent heading",
			from: Cursor{BlockID: "h2-adjacent"},
			want: BlockRange{
				StartInclusive: Cursor{BlockID: "h2-adjacent"},
				EndExclusive:   Cursor{BlockID: "h2-after-adjacent"},
			},
			ok: true,
		},
		{
			name: "lower-level heading ends nested section",
			from: Cursor{BlockID: "h2-after-adjacent"},
			want: BlockRange{
				StartInclusive: Cursor{BlockID: "h2-after-adjacent"},
				EndExclusive:   Cursor{BlockID: "h1-final"},
			},
			ok: true,
		},
		{
			name: "EOF section uses zero end cursor",
			from: Cursor{BlockID: "h1-final"},
			want: BlockRange{
				StartInclusive: Cursor{BlockID: "h1-final"},
			},
			ok: true,
		},
		{
			name: "non-heading anchor in EOF section uses zero end cursor",
			from: Cursor{BlockID: "p-final"},
			want: BlockRange{
				StartInclusive: Cursor{BlockID: "h1-final"},
			},
			ok: true,
		},
		{
			name: "before first heading returns false",
			from: Cursor{BlockID: "intro"},
		},
		{
			name: "zero cursor returns false",
			from: Cursor{},
		},
		{
			name: "missing cursor returns false",
			from: Cursor{BlockID: "missing"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := doc.Blocks()
			got, ok := doc.SectionRange(tt.from)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("SectionRange(%#v) = (%#v, %v), want (%#v, %v)", tt.from, got, ok, tt.want, tt.ok)
			}
			if after := doc.Blocks(); !reflect.DeepEqual(after, before) {
				t.Fatalf("SectionRange mutated document: before %#v after %#v", before, after)
			}
		})
	}
}

func TestSectionRangeNoSectionDocuments(t *testing.T) {
	tests := []struct {
		name   string
		blocks []Block
		from   Cursor
	}{
		{
			name: "empty document",
			from: Cursor{BlockID: "missing"},
		},
		{
			name: "headingless document",
			blocks: []Block{
				{ID: "p0", Kind: BlockKindParagraph},
				{ID: "code", Kind: BlockKindCode},
				{ID: "p1", Kind: BlockKindParagraph},
			},
			from: Cursor{BlockID: "code"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := mustDocument(t, tt.blocks)
			before := doc.Blocks()

			got, ok := doc.SectionRange(tt.from)
			if ok || got != (BlockRange{}) {
				t.Fatalf("SectionRange(%#v) = (%#v, %v), want zero range and false", tt.from, got, ok)
			}
			if after := doc.Blocks(); !reflect.DeepEqual(after, before) {
				t.Fatalf("SectionRange mutated document: before %#v after %#v", before, after)
			}
		})
	}
}

func sectionRangeDocument(t *testing.T) Document {
	t.Helper()

	return mustDocument(t, []Block{
		{ID: "intro", Kind: BlockKindParagraph},
		{ID: "h1", Kind: BlockKindHeading, HeadingLevel: 1},
		{ID: "p-h1", Kind: BlockKindParagraph},
		{ID: "h2", Kind: BlockKindHeading, HeadingLevel: 2},
		{ID: "p-h2", Kind: BlockKindParagraph},
		{ID: "h3", Kind: BlockKindHeading, HeadingLevel: 3},
		{ID: "p-h3", Kind: BlockKindParagraph},
		{ID: "h3b", Kind: BlockKindHeading, HeadingLevel: 3},
		{ID: "p-h3b", Kind: BlockKindParagraph},
		{ID: "h2b", Kind: BlockKindHeading, HeadingLevel: 2},
		{ID: "p-h2b", Kind: BlockKindParagraph},
		{ID: "h1b", Kind: BlockKindHeading, HeadingLevel: 1},
		{ID: "h2-skip", Kind: BlockKindHeading, HeadingLevel: 2},
		{ID: "h5-skip", Kind: BlockKindHeading, HeadingLevel: 5},
		{ID: "p-skip", Kind: BlockKindParagraph},
		{ID: "h2-sibling", Kind: BlockKindHeading, HeadingLevel: 2},
		{ID: "h2-adjacent", Kind: BlockKindHeading, HeadingLevel: 2},
		{ID: "h2-after-adjacent", Kind: BlockKindHeading, HeadingLevel: 2},
		{ID: "p-after-adjacent", Kind: BlockKindParagraph},
		{ID: "h1-final", Kind: BlockKindHeading, HeadingLevel: 1},
		{ID: "p-final", Kind: BlockKindParagraph},
		{ID: "h2-final-child", Kind: BlockKindHeading, HeadingLevel: 2},
	})
}
