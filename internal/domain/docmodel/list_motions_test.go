package docmodel

import (
	"reflect"
	"testing"
)

func TestListNextBlockOfKindSupportsStrictForwardNavigation(t *testing.T) {
	doc := listMotionDocument(t)

	tests := []struct {
		name  string
		from  Cursor
		count int
		want  Cursor
		ok    bool
	}{
		{
			name: "forward list motion skips non lists",
			from: Cursor{BlockID: "h1"},
			want: Cursor{BlockID: "list1"},
			ok:   true,
		},
		{
			name:  "count below one means one",
			from:  Cursor{BlockID: "h1"},
			count: -1,
			want:  Cursor{BlockID: "list1"},
			ok:    true,
		},
		{
			name:  "count zero means one",
			from:  Cursor{BlockID: "code1"},
			count: 0,
			want:  Cursor{BlockID: "list2"},
			ok:    true,
		},
		{
			name:  "strict forward list motion does not match current list",
			from:  Cursor{BlockID: "list1"},
			count: 1,
			want:  Cursor{BlockID: "list2"},
			ok:    true,
		},
		{
			name:  "count selects nth next list",
			from:  Cursor{BlockID: "p0"},
			count: 2,
			want:  Cursor{BlockID: "list2"},
			ok:    true,
		},
		{
			name: "no later list returns false",
			from: Cursor{BlockID: "list3"},
		},
		{
			name:  "count past later lists returns false",
			from:  Cursor{BlockID: "list1"},
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
			got, ok := doc.NextBlockOfKind(tt.from, BlockKindList, tt.count)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("NextBlockOfKind(%#v, BlockKindList, %d) = (%#v, %v), want (%#v, %v)", tt.from, tt.count, got, ok, tt.want, tt.ok)
			}
			if after := doc.Blocks(); !reflect.DeepEqual(after, before) {
				t.Fatalf("NextBlockOfKind mutated document: before %#v after %#v", before, after)
			}
		})
	}
}

func TestListPreviousBlockOfKindSupportsStrictBackwardNavigation(t *testing.T) {
	doc := listMotionDocument(t)

	tests := []struct {
		name  string
		from  Cursor
		count int
		want  Cursor
		ok    bool
	}{
		{
			name: "backward list motion skips non lists",
			from: Cursor{BlockID: "table1"},
			want: Cursor{BlockID: "list3"},
			ok:   true,
		},
		{
			name:  "count below one means one",
			from:  Cursor{BlockID: "table1"},
			count: -1,
			want:  Cursor{BlockID: "list3"},
			ok:    true,
		},
		{
			name:  "count zero means one",
			from:  Cursor{BlockID: "code1"},
			count: 0,
			want:  Cursor{BlockID: "list1"},
			ok:    true,
		},
		{
			name:  "strict backward list motion does not match current list",
			from:  Cursor{BlockID: "list3"},
			count: 1,
			want:  Cursor{BlockID: "list2"},
			ok:    true,
		},
		{
			name:  "count selects nth previous list",
			from:  Cursor{BlockID: "list3"},
			count: 2,
			want:  Cursor{BlockID: "list1"},
			ok:    true,
		},
		{
			name: "no earlier list returns false",
			from: Cursor{BlockID: "list0"},
		},
		{
			name:  "count past earlier lists returns false",
			from:  Cursor{BlockID: "list2"},
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
			got, ok := doc.PreviousBlockOfKind(tt.from, BlockKindList, tt.count)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("PreviousBlockOfKind(%#v, BlockKindList, %d) = (%#v, %v), want (%#v, %v)", tt.from, tt.count, got, ok, tt.want, tt.ok)
			}
			if after := doc.Blocks(); !reflect.DeepEqual(after, before) {
				t.Fatalf("PreviousBlockOfKind mutated document: before %#v after %#v", before, after)
			}
		})
	}
}

func listMotionDocument(t *testing.T) Document {
	t.Helper()

	return mustDocument(t, []Block{
		{ID: "list0", Kind: BlockKindList},
		{ID: "p0", Kind: BlockKindParagraph},
		{ID: "h1", Kind: BlockKindHeading, HeadingLevel: 1},
		{ID: "list1", Kind: BlockKindList},
		{ID: "code1", Kind: BlockKindCode},
		{ID: "list2", Kind: BlockKindList},
		{ID: "image1", Kind: BlockKindImage},
		{ID: "list3", Kind: BlockKindList},
		{ID: "table1", Kind: BlockKindTable},
	})
}
