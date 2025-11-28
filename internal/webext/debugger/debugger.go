// Package debugger provides WebExtension API introspection and functional testing.
// It runs directly via Sobek (Go JS runtime) to validate API implementations.
package debugger

import (
	"fmt"
	"log"
	"strings"

	"github.com/grafana/sobek"
)

const logPrefix = "[webext-debugger]"

// APIPath defines an expected WebExtension API path.
type APIPath struct {
	Path     string // e.g. "browser.storage.local.get"
	Type     string // "function", "object", "string", "number", "boolean"
	Required bool   // true if critical for extensions like uBlock
	Status   string // "implemented", "missing", "stub" - for documentation
}

// TestCase defines a functional test.
type TestCase struct {
	Name    string
	Code    string                                                       // JS code to run
	Check   func(vm *sobek.Runtime, v sobek.Value, err error) (bool, string) // Returns (pass, details)
	Cleanup string                                                       // JS cleanup code (optional)
}

// Result captures a test outcome.
type Result struct {
	Category string // "introspect" or "test"
	Name     string
	Status   string // "EXISTS", "MISSING", "PASS", "FAIL"
	Details  string
	Required bool // highlight if critical API is missing
}

// Summary holds aggregated results.
type Summary struct {
	IntrospectExists  int
	IntrospectMissing int
	RequiredMissing   int
	TestsPass         int
	TestsFail         int
}

// Debugger orchestrates API introspection and functional tests.
type Debugger struct {
	vm      *sobek.Runtime
	extID   string
	results []Result
}

// New creates a new Debugger for the given Sobek runtime.
func New(vm *sobek.Runtime, extID string) *Debugger {
	return &Debugger{
		vm:    vm,
		extID: extID,
	}
}

// Run executes all introspection and functional tests.
func Run(vm *sobek.Runtime, extID string) {
	d := New(vm, extID)
	d.RunAll()
}

// RunAll executes introspection and functional tests, then prints summary.
func (d *Debugger) RunAll() {
	log.Printf("%s === WEBEXT API DEBUGGER for %s ===", logPrefix, d.extID)
	log.Printf("%s", logPrefix)

	// Run introspection
	d.Introspect()

	// Run functional tests
	d.RunTests()

	// Print summary
	d.PrintSummary()
}

// Introspect checks all expected API paths for existence and type.
func (d *Debugger) Introspect() {
	log.Printf("%s --- INTROSPECTION ---", logPrefix)

	for _, api := range ExpectedAPIs {
		exists, actualType := d.checkPath(api.Path)

		status := "MISSING"
		details := ""
		if exists {
			status = "EXISTS"
			details = fmt.Sprintf("(%s)", actualType)
		}

		result := Result{
			Category: "introspect",
			Name:     api.Path,
			Status:   status,
			Details:  details,
			Required: api.Required,
		}
		d.results = append(d.results, result)

		// Log with required marker
		marker := ""
		if api.Required && !exists {
			marker = " [REQUIRED!]"
		} else if exists && api.Required {
			marker = " ✓"
		}
		log.Printf("%s %s: %s %s%s", logPrefix, api.Path, status, details, marker)
	}

	log.Printf("%s", logPrefix)
}

// RunTests executes all functional tests.
func (d *Debugger) RunTests() {
	log.Printf("%s --- FUNCTIONAL TESTS ---", logPrefix)

	for _, test := range FunctionalTests {
		pass, details := d.runTest(test)

		status := "PASS"
		if !pass {
			status = "FAIL"
		}

		result := Result{
			Category: "test",
			Name:     test.Name,
			Status:   status,
			Details:  details,
		}
		d.results = append(d.results, result)

		detailStr := ""
		if details != "" {
			detailStr = " - " + details
		}
		log.Printf("%s %s: %s%s", logPrefix, test.Name, status, detailStr)

		// Run cleanup if provided
		if test.Cleanup != "" {
			_, _ = d.vm.RunString(test.Cleanup)
		}
	}

	log.Printf("%s", logPrefix)
}

// PrintSummary outputs aggregated results.
func (d *Debugger) PrintSummary() {
	summary := d.GetSummary()

	log.Printf("%s === SUMMARY ===", logPrefix)
	log.Printf("%s Introspection: %d exist, %d missing (%d required)",
		logPrefix, summary.IntrospectExists, summary.IntrospectMissing, summary.RequiredMissing)
	log.Printf("%s Functional: %d pass, %d fail",
		logPrefix, summary.TestsPass, summary.TestsFail)
	log.Printf("%s", logPrefix)

	if summary.RequiredMissing == 0 && summary.TestsFail == 0 {
		log.Printf("%s ✓ All required APIs are implemented!", logPrefix)
	} else {
		if summary.RequiredMissing > 0 {
			log.Printf("%s ✗ %d required API(s) missing!", logPrefix, summary.RequiredMissing)
		}
		if summary.TestsFail > 0 {
			log.Printf("%s ✗ %d functional test(s) failed!", logPrefix, summary.TestsFail)
		}
	}
}

// GetSummary returns aggregated results.
func (d *Debugger) GetSummary() Summary {
	var s Summary
	for _, r := range d.results {
		switch r.Category {
		case "introspect":
			if r.Status == "EXISTS" {
				s.IntrospectExists++
			} else {
				s.IntrospectMissing++
				if r.Required {
					s.RequiredMissing++
				}
			}
		case "test":
			if r.Status == "PASS" {
				s.TestsPass++
			} else {
				s.TestsFail++
			}
		}
	}
	return s
}

// checkPath checks if a dotted path exists in the VM and returns its type.
func (d *Debugger) checkPath(path string) (exists bool, jsType string) {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return false, ""
	}

	// Build JS code to check the path
	code := fmt.Sprintf(`(function() {
		try {
			var obj = %s;
			for (var i = 1; i < %d; i++) {
				var parts = %q.split(".");
				obj = obj[parts[i]];
				if (obj === undefined || obj === null) return null;
			}
			return typeof obj;
		} catch(e) {
			return null;
		}
	})()`, parts[0], len(parts), path)

	result, err := d.vm.RunString(code)
	if err != nil || result == nil || sobek.IsNull(result) || sobek.IsUndefined(result) {
		return false, ""
	}

	return true, result.String()
}

// runTest executes a single test case.
func (d *Debugger) runTest(test TestCase) (pass bool, details string) {
	result, err := d.vm.RunString(test.Code)
	return test.Check(d.vm, result, err)
}
