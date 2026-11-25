package homehtml

import "bnema.dev/dumber/internal/application/port"

// Renderer adapts RenderTemplate to port.HomeTemplateRenderer.
type Renderer struct{}

var _ port.HomeTemplateRenderer = Renderer{}

// Render executes tmpl against data and returns sanitized HTML.
func (Renderer) Render(tmpl string, data any) (string, error) {
	return RenderTemplate(tmpl, data)
}
