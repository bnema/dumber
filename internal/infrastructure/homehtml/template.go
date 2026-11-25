package homehtml

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"html/template"
	"strings"
	"sync"
	"time"

	"bnema.dev/dumber/internal/domain/entity"
)

const (
	// MaxTemplateOutputBytes caps the rendered template output.
	MaxTemplateOutputBytes = 64 << 10
	// TemplateTimeout bounds how long a template may execute.
	TemplateTimeout = 2 * time.Second
	// MaxTemplateArrayItems caps every array in the template data. Longer
	// arrays are truncated so nested ranges stay cheap.
	MaxTemplateArrayItems = 500
	// MaxTemplateRangeDepth is how deep ranges may nest. With the array cap,
	// a template loops at most 500^2 times.
	MaxTemplateRangeDepth = 2
)

var (
	// ErrTemplateOutputTooLarge is returned when rendering exceeds MaxTemplateOutputBytes.
	ErrTemplateOutputTooLarge = errors.New("template output exceeds 64 KiB")
	// ErrTemplateTimeout is returned when rendering exceeds TemplateTimeout.
	ErrTemplateTimeout = errors.New("template execution timed out")
	// ErrTemplateIntRange is returned for {{range N}} over a number, which
	// could loop for a very long time.
	ErrTemplateIntRange = errors.New("range over a number is not allowed")
	// ErrTemplateRangeDepth is returned for more than MaxTemplateRangeDepth
	// nested ranges.
	ErrTemplateRangeDepth = errors.New("ranges nested more than 2 deep are not allowed")
)

var templateFuncs = template.FuncMap{
	"format": func(format string, v any) string {
		return entity.FormatHomeField(v, entity.HomeFieldFormat(format))
	},
	"default": func(def, v any) any {
		if isEmpty(v) {
			return def
		}
		return v
	},
	"join": func(sep string, items []any) string {
		parts := make([]string, 0, len(items))
		for _, it := range items {
			parts = append(parts, fmt.Sprint(it))
		}
		return strings.Join(parts, sep)
	},
}

// RenderTemplate executes tmpl (html/template) against data and returns the
// sanitized HTML. Parse errors never include data.
func RenderTemplate(tmpl string, data any) (string, error) {
	return renderTemplate(context.Background(), tmpl, data, TemplateTimeout)
}

// maxCachedTemplates bounds the parsed template cache. A home page has far
// fewer widget templates; old entries go once the cap is reached.
const maxCachedTemplates = 64

// templateCache keeps parsed and checked templates by source hash, so a
// widget refresh does not parse its template again. html/template allows
// concurrent Execute calls on one template.
var templateCache = struct {
	sync.Mutex
	byHash map[[sha256.Size]byte]*template.Template
	order  [][sha256.Size]byte
}{byHash: make(map[[sha256.Size]byte]*template.Template)}

// parseTemplate returns the parsed, range-checked template for src.
func parseTemplate(src string) (*template.Template, error) {
	key := sha256.Sum256([]byte(src))
	templateCache.Lock()
	t, ok := templateCache.byHash[key]
	templateCache.Unlock()
	if ok {
		return t, nil
	}
	t, err := template.New("widget").Funcs(templateFuncs).Parse(src)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	if err := checkRanges(t); err != nil {
		return nil, err
	}
	templateCache.Lock()
	defer templateCache.Unlock()
	if cached, ok := templateCache.byHash[key]; ok {
		return cached, nil
	}
	if len(templateCache.order) >= maxCachedTemplates {
		oldest := templateCache.order[0]
		templateCache.order = templateCache.order[1:]
		delete(templateCache.byHash, oldest)
	}
	templateCache.byHash[key] = t
	templateCache.order = append(templateCache.order, key)
	return t, nil
}

func renderTemplate(ctx context.Context, tmpl string, data any, timeout time.Duration) (string, error) {
	t, err := parseTemplate(tmpl)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	data = capTemplateArrays(data, MaxTemplateArrayItems)
	buf := &cappedBuffer{limit: MaxTemplateOutputBytes}
	done := make(chan error, 1)
	go func() {
		done <- t.Execute(buf, data)
	}()

	select {
	case err := <-done:
		if errors.Is(err, ErrTemplateOutputTooLarge) {
			return "", ErrTemplateOutputTooLarge
		}
		if err != nil {
			return "", fmt.Errorf("execute template: %w", err)
		}
		return Sanitize(buf.String()), nil
	case <-ctx.Done():
		// The goroutine keeps running until Execute returns; the capped buffer
		// bounds its memory and its result is discarded.
		return "", ErrTemplateTimeout
	}
}

// cappedBuffer fails writes once limit bytes are reached.
type cappedBuffer struct {
	b     strings.Builder
	limit int
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if c.b.Len()+len(p) > c.limit {
		return 0, ErrTemplateOutputTooLarge
	}
	return c.b.Write(p)
}

func (c *cappedBuffer) String() string { return c.b.String() }

func isEmpty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case []any:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	default:
		return false
	}
}

// capTemplateArrays returns a copy of v, a json.Unmarshal result, with every
// array truncated to limit elements. Other values are returned as is.
func capTemplateArrays(v any, limit int) any {
	switch x := v.(type) {
	case []any:
		if len(x) > limit {
			x = x[:limit]
		}
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = capTemplateArrays(item, limit)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, item := range x {
			out[k] = capTemplateArrays(item, limit)
		}
		return out
	default:
		return v
	}
}
