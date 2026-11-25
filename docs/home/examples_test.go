// Package home_test checks that the documented home.toml examples stay valid.
package home_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"

	"bnema.dev/dumber/internal/domain/entity"
)

func TestHomeExamplesAreValid(t *testing.T) {
	files, err := filepath.Glob("examples/*.toml")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no examples found")
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var layout entity.HomeLayout
			dec := toml.NewDecoder(bytes.NewReader(data))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&layout); err != nil {
				t.Fatalf("decode: %v", err)
			}
			layout.Normalize()
			if errs := layout.Validate(); len(errs) > 0 {
				t.Fatalf("validate: %v", errs)
			}
		})
	}
}
