// Package homehtml renders home widget templates and sanitizes the result
// with a strict allowlist before it reaches a system view page.
package homehtml

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const (
	// MaxInputBytes caps the HTML accepted by Sanitize. Longer input is truncated.
	MaxInputBytes = 64 << 10
	// MaxNodes caps how many parsed nodes Sanitize keeps.
	MaxNodes = 5000

	// maxPasses bounds the re-sanitize loop that makes Sanitize idempotent.
	maxPasses = 4
)

var allowedTags = setOf(
	"div", "span", "p", "br", "hr", "ul", "ol", "li", "strong", "em", "b", "i",
	"small", "code", "pre", "table", "thead", "tbody", "tr", "th", "td",
	"h1", "h2", "h3", "h4", "h5", "h6", "a", "img", "progress", "meter", "time",
	"abbr", "sup", "sub", "dl", "dt", "dd", "section", "header", "footer",
	"figure", "figcaption",
)

// droppedTags are removed together with their contents.
var droppedTags = setOf(
	"script", "style", "iframe", "object", "embed", "form", "input", "button",
	"textarea", "select", "link", "meta", "base", "svg", "math", "template",
	"noscript", "noembed", "noframes", "xmp", "plaintext", "title", "frame",
	"frameset", "applet", "head",
)

var globalAttrs = setOf("class", "title", "colspan", "rowspan")

var rangeAttrs = setOf("value", "max", "min", "low", "high", "optimum")

var styleProps = setOf(
	"color", "background-color", "width", "height", "max-width", "text-align",
	"font-weight", "font-style", "opacity", "display", "gap", "border-radius",
)

// blockedStyleTokens reject a style value that contains any of them.
var blockedStyleTokens = []string{
	"url(", "expression", "\\", "@", "<", ">", "/*", "*/", "\"", "'",
	"javascript:", "vbscript:", "data:", "&", "{", "}", "image-set", "var(", "attr(",
}

var displayValues = setOf("none", "block", "inline", "flex", "grid")

var imgDataPrefixes = []string{
	"data:image/png", "data:image/gif", "data:image/jpeg", "data:image/webp",
}

// Sanitize returns htmlIn reduced to the allowlisted tags, attributes and
// style properties, re-serialized by html.Render.
func Sanitize(htmlIn string) string {
	if len(htmlIn) > MaxInputBytes {
		htmlIn = htmlIn[:MaxInputBytes]
	}
	out := sanitizeOnce(htmlIn)
	// Unwrapping tags can leave markup the parser rebuilds differently, so
	// repeat until the output is stable.
	for range maxPasses {
		next := sanitizeOnce(out)
		if next == out {
			return out
		}
		out = next
	}
	return out
}

func sanitizeOnce(in string) string {
	ctx := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(strings.NewReader(in), ctx)
	if err != nil {
		return ""
	}
	s := &sanitizer{}
	root := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	for _, n := range nodes {
		s.appendClean(root, n)
	}
	var buf bytes.Buffer
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if err := html.Render(&buf, c); err != nil {
			return ""
		}
	}
	return buf.String()
}

type sanitizer struct {
	nodes int
}

// appendClean appends the sanitized form of n to parent.
func (s *sanitizer) appendClean(parent, n *html.Node) {
	if s.nodes >= MaxNodes {
		return
	}
	s.nodes++
	switch n.Type {
	case html.TextNode:
		parent.AppendChild(&html.Node{Type: html.TextNode, Data: n.Data})
	case html.ElementNode:
		name := strings.ToLower(n.Data)
		if n.Namespace != "" || droppedTags[name] {
			return
		}
		target := parent
		if allowedTags[name] {
			target = &html.Node{
				Type:     html.ElementNode,
				Data:     name,
				DataAtom: atom.Lookup([]byte(name)),
				Attr:     cleanAttrs(name, n.Attr),
			}
			if name == "a" && hasAttr(target.Attr, "href") {
				target.Attr = append(target.Attr,
					html.Attribute{Key: "rel", Val: "noopener noreferrer"},
					html.Attribute{Key: "target", Val: "_blank"})
			}
			parent.AppendChild(target)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			s.appendClean(target, c)
		}
	case html.DocumentNode:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			s.appendClean(parent, c)
		}
	default:
		// Comments, doctypes and raw nodes are dropped.
	}
}

func cleanAttrs(tag string, attrs []html.Attribute) []html.Attribute {
	out := make([]html.Attribute, 0, len(attrs))
	seen := make(map[string]bool, len(attrs))
	for _, a := range attrs {
		if a.Namespace != "" {
			continue
		}
		key := strings.ToLower(a.Key)
		if seen[key] {
			continue
		}
		val, ok := cleanAttr(tag, key, a.Val)
		if !ok {
			continue
		}
		seen[key] = true
		out = append(out, html.Attribute{Key: key, Val: val})
	}
	return out
}

func cleanAttr(tag, key, val string) (string, bool) {
	if strings.HasPrefix(key, "on") {
		return "", false
	}
	switch {
	case key == "style":
		style := cleanStyle(val)
		return style, style != ""
	case tag == "a" && key == "href":
		return cleanURL(val, false)
	case tag == "img" && key == "src":
		return cleanURL(val, true)
	}
	allowed := globalAttrs[key] ||
		isPrefixedAttr(key, "aria-") || isUserDataAttr(key) ||
		((tag == "progress" || tag == "meter") && rangeAttrs[key]) ||
		(tag == "time" && key == "datetime") ||
		(tag == "img" && (key == "alt" || key == "width" || key == "height"))
	if !allowed || hasDangerousScheme(val) {
		return "", false
	}
	return val, true
}

// isUserDataAttr allows data-* except data-sv-*, which the system view page
// reads as actions and regions.
func isUserDataAttr(key string) bool {
	return isPrefixedAttr(key, "data-") && !strings.HasPrefix(key, "data-sv-") && key != "data-sv"
}

func isPrefixedAttr(key, prefix string) bool {
	if !strings.HasPrefix(key, prefix) || len(key) == len(prefix) {
		return false
	}
	for _, r := range key[len(prefix):] {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

// normalize lowercases v and removes whitespace and control characters, the
// way browsers ignore them inside URL schemes. The parser has already decoded
// character references.
func normalize(v string) string {
	var b strings.Builder
	for _, r := range v {
		if r <= 0x20 || r == 0x7f || r == 0xa0 || r == 0xfeff || (r >= 0x200b && r <= 0x200d) {
			continue
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

func hasDangerousScheme(v string) bool {
	n := normalize(v)
	return strings.Contains(n, "javascript:") || strings.Contains(n, "vbscript:") ||
		strings.Contains(n, "data:")
}

func cleanURL(v string, allowImageData bool) (string, bool) {
	n := normalize(v)
	if allowImageData {
		for _, prefix := range imgDataPrefixes {
			if strings.HasPrefix(n, prefix+";") || strings.HasPrefix(n, prefix+",") {
				if strings.Contains(n, "javascript:") || strings.Contains(n, "vbscript:") {
					return "", false
				}
				return strings.TrimSpace(v), true
			}
		}
	}
	if !strings.HasPrefix(n, "http://") && !strings.HasPrefix(n, "https://") {
		return "", false
	}
	if strings.Contains(n, "javascript:") || strings.Contains(n, "vbscript:") {
		return "", false
	}
	return strings.TrimSpace(v), true
}

// cleanStyle keeps only allowlisted declarations with inert values.
func cleanStyle(v string) string {
	decls := make([]string, 0, 4)
	for decl := range strings.SplitSeq(v, ";") {
		prop, value, ok := strings.Cut(decl, ":")
		if !ok {
			continue
		}
		prop = strings.ToLower(strings.TrimSpace(prop))
		value = strings.TrimSpace(value)
		if value == "" || !allowedStyleProp(prop) || !inertStyleValue(value) {
			continue
		}
		if prop == "display" && !displayValues[strings.ToLower(value)] {
			continue
		}
		decls = append(decls, prop+": "+value)
	}
	return strings.Join(decls, "; ")
}

func allowedStyleProp(prop string) bool {
	if styleProps[prop] {
		return true
	}
	for _, base := range []string{"margin", "padding"} {
		if prop == base {
			return true
		}
		switch strings.TrimPrefix(prop, base+"-") {
		case "top", "right", "bottom", "left":
			return strings.HasPrefix(prop, base+"-")
		}
	}
	return false
}

func inertStyleValue(v string) bool {
	n := normalize(v)
	for _, bad := range blockedStyleTokens {
		if strings.Contains(n, bad) {
			return false
		}
	}
	return true
}

func hasAttr(attrs []html.Attribute, key string) bool {
	for _, a := range attrs {
		if a.Key == key {
			return true
		}
	}
	return false
}

func setOf(items ...string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, it := range items {
		m[it] = true
	}
	return m
}
