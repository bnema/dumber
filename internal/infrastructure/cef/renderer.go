package cef

import gtkmenu "github.com/bnema/dumber/internal/infrastructure/gtkmenu"

type Renderer = gtkmenu.Renderer

func NewRenderer(dispatch func(func())) *Renderer {
	return gtkmenu.NewRenderer(dispatch)
}
