package docmodel

import (
	"reflect"
	"testing"
)

func TestStrictNextBlockOfKind(t *testing.T) {
	doc := blockKindMotionDocument(t)

	tests := []struct {
		name  string
		from  Cursor
		kind  BlockKind
		count int
		want  Cursor
		ok    bool
	}{
		{
			name: "next code block",
			from: Cursor{BlockID: "p0"},
			kind: BlockKindCode,
			want: Cursor{BlockID: "code1"},
			ok:   true,
		},
		{
			name: "next table block",
			from: Cursor{BlockID: "p0"},
			kind: BlockKindTable,
			want: Cursor{BlockID: "table1"},
			ok:   true,
		},
		{
			name: "next image block",
			from: Cursor{BlockID: "p0"},
			kind: BlockKindImage,
			want: Cursor{BlockID: "image1"},
			ok:   true,
		},
		{
			name:  "count below one means one",
			from:  Cursor{BlockID: "p1"},
			kind:  BlockKindTable,
			count: -2,
			want:  Cursor{BlockID: "table1"},
			ok:    true,
		},
		{
			name:  "strict next does not match current block",
			from:  Cursor{BlockID: "code1"},
			kind:  BlockKindCode,
			count: 1,
			want:  Cursor{BlockID: "code2"},
			ok:    true,
		},
		{
			name:  "count selects nth next block of kind",
			from:  Cursor{BlockID: "p0"},
			kind:  BlockKindCode,
			count: 2,
			want:  Cursor{BlockID: "code2"},
			ok:    true,
		},
		{
			name: "no later matching kind returns false",
			from: Cursor{BlockID: "code2"},
			kind: BlockKindCode,
		},
		{
			name:  "count past available matches returns false",
			from:  Cursor{BlockID: "p0"},
			kind:  BlockKindImage,
			count: 3,
		},
		{
			name: "zero cursor returns false",
			from: Cursor{},
			kind: BlockKindCode,
		},
		{
			name: "missing cursor returns false",
			from: Cursor{BlockID: "missing"},
			kind: BlockKindCode,
		},
		{
			name: "empty kind returns false",
			from: Cursor{BlockID: "p0"},
			kind: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := doc.Blocks()
			got, ok := doc.NextBlockOfKind(tt.from, tt.kind, tt.count)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("NextBlockOfKind(%#v, %q, %d) = (%#v, %v), want (%#v, %v)", tt.from, tt.kind, tt.count, got, ok, tt.want, tt.ok)
			}
			if after := doc.Blocks(); !reflect.DeepEqual(after, before) {
				t.Fatalf("NextBlockOfKind mutated document: before %#v after %#v", before, after)
			}
		})
	}
}

func TestStrictPreviousBlockOfKind(t *testing.T) {
	doc := blockKindMotionDocument(t)

	tests := []struct {
		name  string
		from  Cursor
		kind  BlockKind
		count int
		want  Cursor
		ok    bool
	}{
		{
			name: "previous code block",
			from: Cursor{BlockID: "p2"},
			kind: BlockKindCode,
			want: Cursor{BlockID: "code2"},
			ok:   true,
		},
		{
			name: "previous table block",
			from: Cursor{BlockID: "p2"},
			kind: BlockKindTable,
			want: Cursor{BlockID: "table2"},
			ok:   true,
		},
		{
			name: "previous image block",
			from: Cursor{BlockID: "p2"},
			kind: BlockKindImage,
			want: Cursor{BlockID: "image2"},
			ok:   true,
		},
		{
			name:  "count below one means one",
			from:  Cursor{BlockID: "p2"},
			kind:  BlockKindTable,
			count: 0,
			want:  Cursor{BlockID: "table2"},
			ok:    true,
		},
		{
			name:  "strict previous does not match current block",
			from:  Cursor{BlockID: "image2"},
			kind:  BlockKindImage,
			count: 1,
			want:  Cursor{BlockID: "image1"},
			ok:    true,
		},
		{
			name:  "count selects nth previous block of kind",
			from:  Cursor{BlockID: "p2"},
			kind:  BlockKindTable,
			count: 2,
			want:  Cursor{BlockID: "table1"},
			ok:    true,
		},
		{
			name: "no earlier matching kind returns false",
			from: Cursor{BlockID: "code1"},
			kind: BlockKindCode,
		},
		{
			name:  "count before available matches returns false",
			from:  Cursor{BlockID: "p2"},
			kind:  BlockKindImage,
			count: 3,
		},
		{
			name: "zero cursor returns false",
			from: Cursor{},
			kind: BlockKindCode,
		},
		{
			name: "missing cursor returns false",
			from: Cursor{BlockID: "missing"},
			kind: BlockKindCode,
		},
		{
			name: "empty kind returns false",
			from: Cursor{BlockID: "p2"},
			kind: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := doc.Blocks()
			got, ok := doc.PreviousBlockOfKind(tt.from, tt.kind, tt.count)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("PreviousBlockOfKind(%#v, %q, %d) = (%#v, %v), want (%#v, %v)", tt.from, tt.kind, tt.count, got, ok, tt.want, tt.ok)
			}
			if after := doc.Blocks(); !reflect.DeepEqual(after, before) {
				t.Fatalf("PreviousBlockOfKind mutated document: before %#v after %#v", before, after)
			}
		})
	}
}

func TestBlockOfKindMotionsOnEmptyAndUnmatchedDocuments(t *testing.T) {
	tests := []struct {
		name   string
		blocks []Block
		from   Cursor
		kind   BlockKind
	}{
		{
			name: "empty document",
			from: Cursor{BlockID: "missing"},
			kind: BlockKindCode,
		},
		{
			name: "document without requested structural kind",
			blocks: []Block{
				{ID: "p0", Kind: BlockKind("paragraph")},
				{ID: "h1", Kind: BlockKindHeading, HeadingLevel: 1},
				{ID: "p1", Kind: BlockKind("list_item")},
			},
			from: Cursor{BlockID: "p0"},
			kind: BlockKindImage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := mustDocument(t, tt.blocks)

			if got, ok := doc.NextBlockOfKind(tt.from, tt.kind, 1); ok || got != (Cursor{}) {
				t.Fatalf("NextBlockOfKind() = (%#v, %v), want zero cursor and false", got, ok)
			}
			if got, ok := doc.PreviousBlockOfKind(tt.from, tt.kind, 1); ok || got != (Cursor{}) {
				t.Fatalf("PreviousBlockOfKind() = (%#v, %v), want zero cursor and false", got, ok)
			}
		})
	}
}

func blockKindMotionDocument(t *testing.T) Document {
	t.Helper()

	return mustDocument(t, []Block{
		{ID: "p0", Kind: BlockKind("paragraph")},
		{ID: "code1", Kind: BlockKindCode},
		{ID: "p1", Kind: BlockKind("paragraph")},
		{ID: "table1", Kind: BlockKindTable},
		{ID: "image1", Kind: BlockKindImage},
		{ID: "code2", Kind: BlockKindCode},
		{ID: "table2", Kind: BlockKindTable},
		{ID: "image2", Kind: BlockKindImage},
		{ID: "p2", Kind: BlockKind("paragraph")},
	})
}
