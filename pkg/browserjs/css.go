package browserjs

import (
	"strings"

	"github.com/grafana/sobek"
)

// CSSManager provides the CSS global object.
type CSSManager struct {
	vm *sobek.Runtime
}

// NewCSSManager creates a new CSS manager.
func NewCSSManager(vm *sobek.Runtime) *CSSManager {
	return &CSSManager{vm: vm}
}

// Install adds the CSS global object.
func (cm *CSSManager) Install() error {
	vm := cm.vm

	css := vm.NewObject()

	// CSS.supports() - in background context, return false
	_ = css.Set("supports", func(call sobek.FunctionCall) sobek.Value {
		return vm.ToValue(false)
	})

	// CSS.escape() - escape string for CSS
	_ = css.Set("escape", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 1 {
			return vm.ToValue("")
		}
		str := call.Arguments[0].String()
		var result strings.Builder
		for _, r := range str {
			switch r {
			case '!', '"', '#', '$', '%', '&', '\'', '(', ')', '*', '+', ',', '.', '/', ':', ';', '<', '=', '>', '?', '@', '[', '\\', ']', '^', '`', '{', '|', '}', '~':
				result.WriteByte('\\')
				result.WriteRune(r)
			default:
				result.WriteRune(r)
			}
		}
		return vm.ToValue(result.String())
	})

	vm.Set("CSS", css)
	return nil
}
