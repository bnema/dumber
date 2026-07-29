package input

import (
	"testing"

	"github.com/bnema/dumber/internal/domain/vimkeys"
	"github.com/bnema/puregotk/v4/gdk"
)

func TestKeyvalToVimKey(t *testing.T) {
	tests := []struct {
		name   string
		keyval uint
		state  gdk.ModifierType
		want   vimkeys.Key
		wantOk bool
	}{
		{
			name:   "braceright with shift encodes glyph without shift mod",
			keyval: uint(gdk.KEY_braceright),
			state:  gdk.ShiftMaskValue,
			want:   vimkeys.Key{Sym: "}"},
			wantOk: true,
		},
		{
			name:   "uppercase J with shift keeps letter and shift",
			keyval: uint(gdk.KEY_J),
			state:  gdk.ShiftMaskValue,
			want:   vimkeys.Key{Sym: "j", Mods: vimkeys.ModShift},
			wantOk: true,
		},
		{
			name:   "lowercase j without mods",
			keyval: uint(gdk.KEY_j),
			state:  gdk.NoModifierMaskValue,
			want:   vimkeys.Key{Sym: "j"},
			wantOk: true,
		},
		{
			name:   "named Escape keeps ctrl modifier",
			keyval: uint(gdk.KEY_Escape),
			state:  gdk.ControlMaskValue,
			want:   vimkeys.Key{Sym: "Esc", Mods: vimkeys.ModCtrl},
			wantOk: true,
		},
		{
			name:   "named Return keeps shift modifier",
			keyval: uint(gdk.KEY_Return),
			state:  gdk.ShiftMaskValue,
			want:   vimkeys.Key{Sym: "CR", Mods: vimkeys.ModShift},
			wantOk: true,
		},
		{
			name:   "named KP_Enter maps to CR",
			keyval: uint(gdk.KEY_KP_Enter),
			state:  gdk.NoModifierMaskValue,
			want:   vimkeys.Key{Sym: "CR"},
			wantOk: true,
		},
		{
			name:   "named Space keeps alt modifier",
			keyval: uint(gdk.KEY_space),
			state:  gdk.AltMaskValue,
			want:   vimkeys.Key{Sym: "Space", Mods: vimkeys.ModAlt},
			wantOk: true,
		},
		{
			name:   "named Tab maps to Tab",
			keyval: uint(gdk.KEY_Tab),
			state:  gdk.NoModifierMaskValue,
			want:   vimkeys.Key{Sym: "Tab"},
			wantOk: true,
		},
		{
			name:   "named ISO_Left_Tab maps to Tab",
			keyval: uint(gdk.KEY_ISO_Left_Tab),
			state:  gdk.ShiftMaskValue,
			want:   vimkeys.Key{Sym: "Tab", Mods: vimkeys.ModShift},
			wantOk: true,
		},
		{
			name:   "named BackSpace is BackSpace not BS",
			keyval: uint(gdk.KEY_BackSpace),
			state:  gdk.NoModifierMaskValue,
			want:   vimkeys.Key{Sym: "BackSpace"},
			wantOk: true,
		},
		{
			name:   "named Delete is Delete not Del",
			keyval: uint(gdk.KEY_Delete),
			state:  gdk.ControlMaskValue,
			want:   vimkeys.Key{Sym: "Delete", Mods: vimkeys.ModCtrl},
			wantOk: true,
		},
		{
			name:   "named Left keeps ctrl",
			keyval: uint(gdk.KEY_Left),
			state:  gdk.ControlMaskValue,
			want:   vimkeys.Key{Sym: "Left", Mods: vimkeys.ModCtrl},
			wantOk: true,
		},
		{
			name:   "named Right",
			keyval: uint(gdk.KEY_Right),
			state:  gdk.NoModifierMaskValue,
			want:   vimkeys.Key{Sym: "Right"},
			wantOk: true,
		},
		{
			name:   "named Up",
			keyval: uint(gdk.KEY_Up),
			state:  gdk.NoModifierMaskValue,
			want:   vimkeys.Key{Sym: "Up"},
			wantOk: true,
		},
		{
			name:   "named Down",
			keyval: uint(gdk.KEY_Down),
			state:  gdk.NoModifierMaskValue,
			want:   vimkeys.Key{Sym: "Down"},
			wantOk: true,
		},
		{
			name:   "named Home",
			keyval: uint(gdk.KEY_Home),
			state:  gdk.NoModifierMaskValue,
			want:   vimkeys.Key{Sym: "Home"},
			wantOk: true,
		},
		{
			name:   "named End",
			keyval: uint(gdk.KEY_End),
			state:  gdk.NoModifierMaskValue,
			want:   vimkeys.Key{Sym: "End"},
			wantOk: true,
		},
		{
			name:   "named PageUp",
			keyval: uint(gdk.KEY_Page_Up),
			state:  gdk.NoModifierMaskValue,
			want:   vimkeys.Key{Sym: "PageUp"},
			wantOk: true,
		},
		{
			name:   "named PageDown",
			keyval: uint(gdk.KEY_Page_Down),
			state:  gdk.AltMaskValue,
			want:   vimkeys.Key{Sym: "PageDown", Mods: vimkeys.ModAlt},
			wantOk: true,
		},
		{
			name:   "ctrl+d printable letter keeps ctrl",
			keyval: uint(gdk.KEY_d),
			state:  gdk.ControlMaskValue,
			want:   vimkeys.Key{Sym: "d", Mods: vimkeys.ModCtrl},
			wantOk: true,
		},
		{
			name:   "non-printable rejected",
			keyval: 0,
			state:  gdk.NoModifierMaskValue,
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+"_roundtrip_string", func(t *testing.T) {
			if !tt.wantOk {
				return
			}
			got, ok := KeyvalToVimKey(tt.keyval, tt.state)
			if !ok {
				t.Fatalf("KeyvalToVimKey() ok = false")
			}
			canonical := got.String()
			parsed, err := vimkeys.ParseBinding(canonical)
			if err != nil {
				t.Fatalf("ParseBinding(%q) error = %v", canonical, err)
			}
			if !parsed.Equal(vimkeys.Sequence{got}) {
				t.Fatalf("round-trip %q -> %v, want %v", canonical, parsed, vimkeys.Sequence{got})
			}
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := KeyvalToVimKey(tt.keyval, tt.state)
			if ok != tt.wantOk {
				t.Fatalf("KeyvalToVimKey() ok = %v, want %v", ok, tt.wantOk)
			}
			if !tt.wantOk {
				return
			}
			if got != tt.want {
				t.Fatalf("KeyvalToVimKey() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
