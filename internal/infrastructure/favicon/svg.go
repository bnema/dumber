package favicon

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"io"

	appport "github.com/bnema/dumber/internal/application/port"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// svgRasterSize is the square size SVG favicons are rendered at before the
// normal PNG normalization and resizing steps.
const svgRasterSize = 128

// rasterizeSVG renders an SVG favicon to a square RGBA image. The parser only
// reads the given bytes: it never resolves external references.
func rasterizeSVG(data []byte) (img image.Image, err error) {
	// The SVG parser handles untrusted page content; a parser panic must
	// degrade to a favicon miss instead of crashing the browser.
	defer func() {
		if r := recover(); r != nil {
			img = nil
			err = fmt.Errorf("%w: svg favicon render panic: %v", appport.ErrFaviconMiss, r)
		}
	}()
	trimmed := bytes.TrimSpace(data)
	if !bytes.Contains(trimmed, []byte("<svg")) {
		return nil, fmt.Errorf("%w: malformed svg", appport.ErrFaviconMiss)
	}
	if validateErr := validateSVGStructure(trimmed); validateErr != nil {
		return nil, validateErr
	}
	icon, err := oksvg.ReadIconStream(bytes.NewReader(trimmed), oksvg.IgnoreErrorMode)
	if err != nil {
		return nil, fmt.Errorf("%w: parse svg favicon: %w", appport.ErrFaviconMiss, err)
	}
	if icon.ViewBox.W <= 0 || icon.ViewBox.H <= 0 {
		return nil, fmt.Errorf("%w: svg favicon has no usable size", appport.ErrFaviconMiss)
	}
	dst := image.NewRGBA(image.Rect(0, 0, svgRasterSize, svgRasterSize))
	icon.SetTarget(0, 0, svgRasterSize, svgRasterSize)
	scanner := rasterx.NewScannerGV(svgRasterSize, svgRasterSize, dst, dst.Bounds())
	icon.Draw(rasterx.NewDasher(svgRasterSize, svgRasterSize, scanner), 1)
	if isFullyTransparent(dst) {
		return nil, fmt.Errorf("%w: svg favicon rendered empty", appport.ErrFaviconMiss)
	}
	return dst, nil
}

const (
	// maxSVGUseElements bounds <use> expansion work for one favicon.
	maxSVGUseElements = 64
	// maxSVGDepth bounds element nesting depth.
	maxSVGDepth = 64
)

// validateSVGStructure rejects SVGs that could make the renderer recurse
// without bound. oksvg expands <use> references recursively with no cycle
// detection, and a runtime stack overflow cannot be recovered. oksvg only
// resolves <use> targets from <defs> content, so forbidding <use> inside
// <defs> limits expansion to one level and rules out cycles. The decoder
// is strict like oksvg's, so both see the same element tree.
func validateSVGStructure(data []byte) error {
	dec := xml.NewDecoder(bytes.NewReader(data))
	depth, defsDepth, uses := 0, 0, 0
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("%w: malformed svg: %w", appport.ErrFaviconMiss, err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if depth > maxSVGDepth {
				return fmt.Errorf("%w: svg favicon nested too deeply", appport.ErrFaviconMiss)
			}
			switch t.Name.Local {
			case "defs":
				if defsDepth == 0 {
					defsDepth = depth
				}
			case "use":
				if defsDepth > 0 {
					return fmt.Errorf("%w: svg favicon has <use> inside <defs>", appport.ErrFaviconMiss)
				}
				uses++
				if uses > maxSVGUseElements {
					return fmt.Errorf("%w: svg favicon has too many <use> elements", appport.ErrFaviconMiss)
				}
			}
		case xml.EndElement:
			if depth == defsDepth {
				defsDepth = 0
			}
			depth--
		}
	}
	return nil
}

func isFullyTransparent(img *image.RGBA) bool {
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 0 {
			return false
		}
	}
	return true
}
