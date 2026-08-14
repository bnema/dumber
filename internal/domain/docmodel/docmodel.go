package docmodel

import (
	"errors"
	"fmt"
)

// BlockID identifies a block within a document.
type BlockID string

// BlockKind describes the semantic role of a block. Non-heading kinds are
// intentionally left schema-agnostic and are preserved as provided.
type BlockKind string

const (
	// BlockKindHeading marks blocks that participate in heading motions.
	BlockKindHeading BlockKind = "heading"
	// BlockKindParagraph marks a paragraph block for Vim Mode paragraph motions.
	BlockKindParagraph BlockKind = "paragraph"
	// BlockKindCode marks a structural code block for Vim Mode block-kind motions.
	BlockKindCode BlockKind = "code"
	// BlockKindTable marks a structural table block for Vim Mode block-kind motions.
	BlockKindTable BlockKind = "table"
	// BlockKindImage marks a structural image block for Vim Mode block-kind motions.
	BlockKindImage BlockKind = "image"

	// MinHeadingLevel is the lowest valid heading level.
	MinHeadingLevel = 1
	// MaxHeadingLevel is the highest valid heading level.
	MaxHeadingLevel = 6
)

var (
	// ErrEmptyBlockID reports a block with no ID.
	ErrEmptyBlockID = errors.New("docmodel: empty block ID")
	// ErrDuplicateBlockID reports a repeated block ID.
	ErrDuplicateBlockID = errors.New("docmodel: duplicate block ID")
	// ErrInvalidHeadingLevel reports a heading outside levels 1 through 6.
	ErrInvalidHeadingLevel = errors.New("docmodel: invalid heading level")
	// ErrUnexpectedHeadingLevel reports a non-heading block with a heading level.
	ErrUnexpectedHeadingLevel = errors.New("docmodel: non-heading block has heading level")
)

// Block is an ordered document block. HeadingLevel is meaningful only when
// Kind is BlockKindHeading; non-heading blocks must leave it as zero.
type Block struct {
	ID           BlockID
	Kind         BlockKind
	HeadingLevel int
}

// IsHeading reports whether the block is a heading.
func (b Block) IsHeading() bool {
	return b.Kind == BlockKindHeading
}

// Cursor identifies the current block for document navigation.
type Cursor struct {
	BlockID BlockID
}

// Document is an immutable, order-preserving collection of blocks.
type Document struct {
	blocks    []Block
	indexByID map[BlockID]int
}

// NewDocument validates blocks and returns an order-preserving document. The
// input slice is copied so later caller mutations cannot affect navigation.
func NewDocument(blocks []Block) (Document, error) {
	copied := append([]Block(nil), blocks...)
	indexByID := make(map[BlockID]int, len(copied))

	for i, block := range copied {
		if block.ID == "" {
			return Document{}, fmt.Errorf("%w at index %d", ErrEmptyBlockID, i)
		}
		if _, exists := indexByID[block.ID]; exists {
			return Document{}, fmt.Errorf("%w %q at index %d", ErrDuplicateBlockID, block.ID, i)
		}
		if err := validateBlockHeading(block, i); err != nil {
			return Document{}, err
		}

		indexByID[block.ID] = i
	}

	return Document{
		blocks:    copied,
		indexByID: indexByID,
	}, nil
}

// Len returns the number of blocks in document order.
func (d Document) Len() int {
	return len(d.blocks)
}

// Blocks returns a copy of the blocks in document order.
func (d Document) Blocks() []Block {
	blocks := make([]Block, len(d.blocks))
	copy(blocks, d.blocks)
	return blocks
}

// BlockByID returns the block with id when present.
func (d Document) BlockByID(id BlockID) (Block, bool) {
	idx, ok := d.indexByID[id]
	if !ok {
		return Block{}, false
	}
	return d.blocks[idx], true
}

// CursorByID returns a cursor positioned on id when present.
func (d Document) CursorByID(id BlockID) (Cursor, bool) {
	if _, ok := d.indexByID[id]; !ok {
		return Cursor{}, false
	}
	return Cursor{BlockID: id}, true
}

// FirstCursor returns a cursor positioned on the first block in document order.
func (d Document) FirstCursor() (Cursor, bool) {
	if len(d.blocks) == 0 {
		return Cursor{}, false
	}
	return Cursor{BlockID: d.blocks[0].ID}, true
}

func validateBlockHeading(block Block, index int) error {
	if block.IsHeading() {
		if block.HeadingLevel < MinHeadingLevel || block.HeadingLevel > MaxHeadingLevel {
			return fmt.Errorf("%w %d at index %d", ErrInvalidHeadingLevel, block.HeadingLevel, index)
		}
		return nil
	}

	if block.HeadingLevel != 0 {
		return fmt.Errorf("%w %d at index %d", ErrUnexpectedHeadingLevel, block.HeadingLevel, index)
	}
	return nil
}
