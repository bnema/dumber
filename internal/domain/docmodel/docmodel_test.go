package docmodel

import (
	"errors"
	"reflect"
	"testing"
)

func TestNewDocumentValidation(t *testing.T) {
	tests := []struct {
		name    string
		blocks  []Block
		wantErr error
	}{
		{
			name: "empty document is valid",
		},
		{
			name: "valid headings and non headings",
			blocks: []Block{
				{ID: "h1", Kind: BlockKindHeading, HeadingLevel: 1},
				{ID: "body", Kind: BlockKind("paragraph")},
				{ID: "h6", Kind: BlockKindHeading, HeadingLevel: 6},
			},
		},
		{
			name: "empty block ID is invalid",
			blocks: []Block{
				{ID: "ok", Kind: BlockKind("paragraph")},
				{ID: "", Kind: BlockKindHeading, HeadingLevel: 1},
			},
			wantErr: ErrEmptyBlockID,
		},
		{
			name: "duplicate block ID is invalid",
			blocks: []Block{
				{ID: "same", Kind: BlockKindHeading, HeadingLevel: 1},
				{ID: "same", Kind: BlockKind("paragraph")},
			},
			wantErr: ErrDuplicateBlockID,
		},
		{
			name: "heading level below one is invalid",
			blocks: []Block{
				{ID: "h0", Kind: BlockKindHeading, HeadingLevel: 0},
			},
			wantErr: ErrInvalidHeadingLevel,
		},
		{
			name: "heading level above six is invalid",
			blocks: []Block{
				{ID: "h7", Kind: BlockKindHeading, HeadingLevel: 7},
			},
			wantErr: ErrInvalidHeadingLevel,
		},
		{
			name: "non heading cannot carry heading level",
			blocks: []Block{
				{ID: "body", Kind: BlockKind("paragraph"), HeadingLevel: 2},
			},
			wantErr: ErrUnexpectedHeadingLevel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewDocument(tt.blocks)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("NewDocument() error = %v, want nil", err)
				}
				return
			}

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewDocument() error = %v, want errors.Is(_, %v)", err, tt.wantErr)
			}
		})
	}
}

func TestDocumentOrderAndCopies(t *testing.T) {
	blocks := []Block{
		{ID: "intro", Kind: BlockKindHeading, HeadingLevel: 1},
		{ID: "body", Kind: BlockKind("paragraph")},
		{ID: "details", Kind: BlockKindHeading, HeadingLevel: 2},
	}

	doc, err := NewDocument(blocks)
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}

	blocks[0] = Block{ID: "mutated", Kind: BlockKindHeading, HeadingLevel: 6}

	got := doc.Blocks()
	want := []Block{
		{ID: "intro", Kind: BlockKindHeading, HeadingLevel: 1},
		{ID: "body", Kind: BlockKind("paragraph")},
		{ID: "details", Kind: BlockKindHeading, HeadingLevel: 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Blocks() = %#v, want %#v", got, want)
	}

	got[0] = Block{ID: "also-mutated", Kind: BlockKind("paragraph")}
	gotAgain := doc.Blocks()
	if !reflect.DeepEqual(gotAgain, want) {
		t.Fatalf("Blocks() after mutating returned slice = %#v, want %#v", gotAgain, want)
	}
}

func TestDocumentCursorNavigation(t *testing.T) {
	t.Run("empty document has no first cursor", func(t *testing.T) {
		doc, err := NewDocument(nil)
		if err != nil {
			t.Fatalf("NewDocument() error = %v", err)
		}

		if got, ok := doc.FirstCursor(); ok || got != (Cursor{}) {
			t.Fatalf("FirstCursor() = (%#v, %v), want zero cursor and false", got, ok)
		}
	})

	t.Run("cursor by ID and first cursor use document order", func(t *testing.T) {
		doc := mustDocument(t, []Block{
			{ID: "first", Kind: BlockKind("paragraph")},
			{ID: "second", Kind: BlockKindHeading, HeadingLevel: 2},
		})

		if got, ok := doc.FirstCursor(); !ok || got != (Cursor{BlockID: "first"}) {
			t.Fatalf("FirstCursor() = (%#v, %v), want first cursor and true", got, ok)
		}

		if got, ok := doc.CursorByID("second"); !ok || got != (Cursor{BlockID: "second"}) {
			t.Fatalf("CursorByID(second) = (%#v, %v), want second cursor and true", got, ok)
		}

		if got, ok := doc.CursorByID("missing"); ok || got != (Cursor{}) {
			t.Fatalf("CursorByID(missing) = (%#v, %v), want zero cursor and false", got, ok)
		}
	})
}

func TestStrictNextHeading(t *testing.T) {
	doc := headingMotionDocument(t)

	tests := []struct {
		name   string
		from   BlockID
		count  int
		levels []int
		want   Cursor
		ok     bool
	}{
		{
			name: "from non heading uses next heading",
			from: "p0",
			want: Cursor{BlockID: "h1"},
			ok:   true,
		},
		{
			name:  "count below one means one",
			from:  "p1",
			count: -3,
			want:  Cursor{BlockID: "h2"},
			ok:    true,
		},
		{
			name:  "strict next does not match current heading",
			from:  "h1",
			count: 1,
			want:  Cursor{BlockID: "h2"},
			ok:    true,
		},
		{
			name:  "count selects nth next heading",
			from:  "h1",
			count: 2,
			want:  Cursor{BlockID: "h3"},
			ok:    true,
		},
		{
			name:   "level filter skips other headings",
			from:   "h1",
			count:  1,
			levels: []int{1},
			want:   Cursor{BlockID: "h1b"},
			ok:     true,
		},
		{
			name:   "count applies after filtering",
			from:   "h1",
			count:  2,
			levels: []int{2},
			want:   Cursor{BlockID: "h2b"},
			ok:     true,
		},
		{
			name:   "multi level filter matches any listed level",
			from:   "h2",
			count:  1,
			levels: []int{1, 3},
			want:   Cursor{BlockID: "h3"},
			ok:     true,
		},
		{
			name:   "no matching level returns false",
			from:   "p0",
			levels: []int{6},
		},
		{
			name: "past last heading returns false",
			from: "h1b",
		},
		{
			name: "missing cursor returns false",
			from: "missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := doc.Blocks()
			got, ok := doc.NextHeading(Cursor{BlockID: tt.from}, tt.count, tt.levels...)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("NextHeading(%q, %d, %v) = (%#v, %v), want (%#v, %v)", tt.from, tt.count, tt.levels, got, ok, tt.want, tt.ok)
			}
			if after := doc.Blocks(); !reflect.DeepEqual(after, before) {
				t.Fatalf("NextHeading mutated document: before %#v after %#v", before, after)
			}
		})
	}
}

func TestStrictPreviousHeading(t *testing.T) {
	doc := headingMotionDocument(t)

	tests := []struct {
		name   string
		from   BlockID
		count  int
		levels []int
		want   Cursor
		ok     bool
	}{
		{
			name: "from non heading uses previous heading",
			from: "p2",
			want: Cursor{BlockID: "h2b"},
			ok:   true,
		},
		{
			name:  "count below one means one",
			from:  "p2",
			count: 0,
			want:  Cursor{BlockID: "h2b"},
			ok:    true,
		},
		{
			name:  "strict previous does not match current heading",
			from:  "h1b",
			count: 1,
			want:  Cursor{BlockID: "h2b"},
			ok:    true,
		},
		{
			name:  "count selects nth previous heading",
			from:  "h1b",
			count: 2,
			want:  Cursor{BlockID: "h3"},
			ok:    true,
		},
		{
			name:   "level filter skips other headings",
			from:   "h1b",
			count:  1,
			levels: []int{1},
			want:   Cursor{BlockID: "h1"},
			ok:     true,
		},
		{
			name:   "count applies after filtering",
			from:   "h1b",
			count:  2,
			levels: []int{2},
			want:   Cursor{BlockID: "h2"},
			ok:     true,
		},
		{
			name:   "multi level filter matches any listed level",
			from:   "h2b",
			count:  1,
			levels: []int{1, 3},
			want:   Cursor{BlockID: "h3"},
			ok:     true,
		},
		{
			name:   "no matching level returns false",
			from:   "h1b",
			levels: []int{6},
		},
		{
			name: "before first heading returns false",
			from: "h1",
		},
		{
			name: "missing cursor returns false",
			from: "missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := doc.Blocks()
			got, ok := doc.PreviousHeading(Cursor{BlockID: tt.from}, tt.count, tt.levels...)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("PreviousHeading(%q, %d, %v) = (%#v, %v), want (%#v, %v)", tt.from, tt.count, tt.levels, got, ok, tt.want, tt.ok)
			}
			if after := doc.Blocks(); !reflect.DeepEqual(after, before) {
				t.Fatalf("PreviousHeading mutated document: before %#v after %#v", before, after)
			}
		})
	}
}

func TestHeadingMotionsOnEmptyAndHeadinglessDocuments(t *testing.T) {
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
			name: "document without headings",
			blocks: []Block{
				{ID: "p0", Kind: BlockKind("paragraph")},
				{ID: "p1", Kind: BlockKind("list_item")},
			},
			from: Cursor{BlockID: "p0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := mustDocument(t, tt.blocks)

			if got, ok := doc.NextHeading(tt.from, 1); ok || got != (Cursor{}) {
				t.Fatalf("NextHeading() = (%#v, %v), want zero cursor and false", got, ok)
			}
			if got, ok := doc.PreviousHeading(tt.from, 1); ok || got != (Cursor{}) {
				t.Fatalf("PreviousHeading() = (%#v, %v), want zero cursor and false", got, ok)
			}
		})
	}
}

func headingMotionDocument(t *testing.T) Document {
	t.Helper()

	return mustDocument(t, []Block{
		{ID: "p0", Kind: BlockKindParagraph},
		{ID: "h1", Kind: BlockKindHeading, HeadingLevel: 1},
		{ID: "p1", Kind: BlockKindParagraph},
		{ID: "h2", Kind: BlockKindHeading, HeadingLevel: 2},
		{ID: "h3", Kind: BlockKindHeading, HeadingLevel: 3},
		{ID: "h2b", Kind: BlockKindHeading, HeadingLevel: 2},
		{ID: "p2", Kind: BlockKindParagraph},
		{ID: "h1b", Kind: BlockKindHeading, HeadingLevel: 1},
	})
}

func mustDocument(t *testing.T, blocks []Block) Document {
	t.Helper()

	doc, err := NewDocument(blocks)
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	return doc
}
