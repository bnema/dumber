package homehtml

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"strings"
	"time"

	"bnema.dev/dumber/internal/domain/entity"
)

const (
	// MaxTemplateOutputBytes caps the rendered template output.
	MaxTemplateOutputBytes = 64 << 10
	// TemplateTimeout bounds how long a template may execute.
	TemplateTimeout = 2 * time.Second
)

var (
	// ErrTemplateOutputTooLarge is returned when rendering exceeds MaxTemplateOutputBytes.
	ErrTemplateOutputTooLarge = errors.New("template output exceeds 64 KiB")
	// ErrTemplateTimeout is returned when rendering exceeds TemplateTimeout.
	ErrTemplateTimeout = errors.New("template execution timed out")
	// ErrTemplateIntRange is returned for {{range N}} over a number, which
	// could loop for a very long time.
	ErrTemplateIntRange = errors.New("range over a number is not allowed")
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

func renderTemplate(ctx context.Context, tmpl string, data any, timeout time.Duration) (string, error) {
	t, err := template.New("widget").Funcs(templateFuncs).Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}
	if err := checkRanges(t); err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

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
