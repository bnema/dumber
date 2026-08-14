package docmodel

import (
	"reflect"
	"testing"
)

func TestParagraphNextBlockOfKindProvidesVimCloseBraceSemantics(t *testing.T) {
	doc := paragraphMotionDocument(t)

	tests := []struct {
		name  string
		from  Cursor
		count int
		want  Cursor
		ok    bool
	}{
		{
			name: "forward paragraph motion skips non paragraphs",
			from: Cursor{BlockID: "h1"},
			want: Cursor{BlockID: "p1"},
			ok:   true,
		},
		{
			name:  "count below one means one",
			from:  Cursor{BlockID: "h1"},
			count: -1,
			want:  Cursor{BlockID: "p1"},
			ok:    true,
		},
		{
			name:  "count zero means one",
			from:  Cursor{BlockID: "code1"},
			count: 0,
			want:  Cursor{BlockID: "p2"},
			ok:    true,
		},
		{
			name:  "strict forward paragraph motion does not match current paragraph",
			from:  Cursor{BlockID: "p1"},
			count: 1,
			want:  Cursor{BlockID: "p2"},
			ok:    true,
		},
		{
			name:  "count selects nth next paragraph",
			from:  Cursor{BlockID: "p0"},
			count: 2,
			want:  Cursor{BlockID: "p2"},
			ok:    true,
		},
		{
			name: "no later paragraph returns false",
			from: Cursor{BlockID: "p3"},
		},
		{
			name:  "count past later paragraphs returns false",
			from:  Cursor{BlockID: "p1"},
			count: 3,
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
			got, ok := doc.NextBlockOfKind(tt.from, BlockKindParagraph, tt.count)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("NextBlockOfKind(%#v, BlockKindParagraph, %d) = (%#v, %v), want (%#v, %v)", tt.from, tt.count, got, ok, tt.want, tt.ok)
			}
			if after := doc.Blocks(); !reflect.DeepEqual(after, before) {
				t.Fatalf("NextBlockOfKind mutated document: before %#v after %#v", before, after)
			}
		})
	}
}

func TestParagraphPreviousBlockOfKindProvidesVimOpenBraceSemantics(t *testing.T) {
	doc := paragraphMotionDocument(t)

	tests := []struct {
		name  string
		from  Cursor
		count int
		want  Cursor
		ok    bool
	}{
		{
			name: "backward paragraph motion skips non paragraphs",
			from: Cursor{BlockID: "table1"},
			want: Cursor{BlockID: "p3"},
			ok:   true,
		},
		{
			name:  "count below one means one",
			from:  Cursor{BlockID: "table1"},
			count: -1,
			want:  Cursor{BlockID: "p3"},
			ok:    true,
		},
		{
			name:  "count zero means one",
			from:  Cursor{BlockID: "code1"},
			count: 0,
			want:  Cursor{BlockID: "p1"},
			ok:    true,
		},
		{
			name:  "strict backward paragraph motion does not match current paragraph",
			from:  Cursor{BlockID: "p3"},
			count: 1,
			want:  Cursor{BlockID: "p2"},
			ok:    true,
		},
		{
			name:  "count selects nth previous paragraph",
			from:  Cursor{BlockID: "p3"},
			count: 2,
			want:  Cursor{BlockID: "p1"},
			ok:    true,
		},
		{
			name: "no earlier paragraph returns false",
			from: Cursor{BlockID: "p0"},
		},
		{
			name:  "count past earlier paragraphs returns false",
			from:  Cursor{BlockID: "p2"},
			count: 3,
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
			got, ok := doc.PreviousBlockOfKind(tt.from, BlockKindParagraph, tt.count)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("PreviousBlockOfKind(%#v, BlockKindParagraph, %d) = (%#v, %v), want (%#v, %v)", tt.from, tt.count, got, ok, tt.want, tt.ok)
			}
			if after := doc.Blocks(); !reflect.DeepEqual(after, before) {
				t.Fatalf("PreviousBlockOfKind mutated document: before %#v after %#v", before, after)
			}
		})
	}
}

func paragraphMotionDocument(t *testing.T) Document {
	t.Helper()

	return mustDocument(t, []Block{
		{ID: "p0", Kind: BlockKindParagraph},
		{ID: "h1", Kind: BlockKindHeading, HeadingLevel: 1},
		{ID: "p1", Kind: BlockKindParagraph},
		{ID: "code1", Kind: BlockKindCode},
		{ID: "p2", Kind: BlockKindParagraph},
		{ID: "image1", Kind: BlockKindImage},
		{ID: "p3", Kind: BlockKindParagraph},
		{ID: "table1", Kind: BlockKindTable},
	})
}
