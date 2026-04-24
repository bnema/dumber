package systemviews

import (
	"bytes"
	"context"

	"github.com/a-h/templ"
)

func renderComponent(ctx context.Context, component templ.Component) (string, error) {
	var buf bytes.Buffer
	if component == nil {
		return "", nil
	}
	if err := component.Render(ctx, &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func mustRenderComponent(component templ.Component) string {
	html, err := renderComponent(context.Background(), component)
	if err != nil {
		return errorStateHTML(err.Error())
	}
	return html
}
