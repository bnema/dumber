package filtering

import (
	"bufio"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bnema/dumber/internal/filtering/converter"
	"github.com/bnema/dumber/internal/logging"
)

// DefaultFilterCompiler implements FilterCompiler using the converter package
type DefaultFilterCompiler struct {
	converter *converter.FilterConverter
}

// NewDefaultFilterCompiler creates a new default filter compiler
func NewDefaultFilterCompiler() *DefaultFilterCompiler {
	return &DefaultFilterCompiler{
		converter: converter.NewFilterConverter(),
	}
}

// CompileFromSources downloads and compiles filters from multiple URLs
func (dfc *DefaultFilterCompiler) CompileFromSources(ctx context.Context, sources []string) (*CompiledFilters, error) {
	compiled := NewCompiledFilters()

	for _, url := range sources {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		filters, err := dfc.downloadAndCompile(ctx, url)
		if err != nil {
			logging.Error(fmt.Sprintf("Failed to compile from %s: %v", url, err))
			continue
		}

		compiled.Merge(filters)
	}

	compiled.Version = fmt.Sprintf("compiled-%d", time.Now().Unix())
	compiled.CompiledAt = time.Now()

	return compiled, nil
}

// CompileFromData compiles filters from raw data bytes
func (dfc *DefaultFilterCompiler) CompileFromData(data []byte) (*CompiledFilters, error) {
	compiled := NewCompiledFilters()

	// Reset converter for new compilation
	dfc.converter = converter.NewFilterConverter()

	// Parse line by line
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	lineCount := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineCount++

		// Convert each line
		if err := dfc.converter.ConvertEasyListLine(line); err != nil {
			logging.Debug(fmt.Sprintf("Failed to convert filter line: %s, error: %v", line, err))
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading filter data: %w", err)
	}

	// Get compiled rules from converter
	compiled.NetworkRules = dfc.converter.GetNetworkRules()
	compiled.CosmeticRules = dfc.converter.GetCosmeticRules()
	compiled.GenericHiding = dfc.converter.GetGenericHiding()
	compiled.CompiledAt = time.Now()
	compiled.Version = fmt.Sprintf("data-compile-%d", time.Now().Unix())

	logging.Info(fmt.Sprintf("Compiled %d filter lines into %d network rules and %d cosmetic rules",
		lineCount, len(compiled.NetworkRules), len(compiled.CosmeticRules)))

	return compiled, nil
}

// downloadAndCompile downloads and compiles a single filter list
func (dfc *DefaultFilterCompiler) downloadAndCompile(ctx context.Context, url string) (*CompiledFilters, error) {
	// This would implement HTTP download - for now, return empty filters
	logging.Debug(fmt.Sprintf("Would download and compile from %s", url))

	compiled := NewCompiledFilters()
	compiled.Version = fmt.Sprintf("stub-%d", time.Now().Unix())
	compiled.CompiledAt = time.Now()

	return compiled, nil
}
