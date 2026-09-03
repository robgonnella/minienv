package errs_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// declaredKind is one errs.Kind constant found in an internal/*/error.go.
type declaredKind struct {
	pkg   string // owning package directory, e.g. "config"
	name  string // constant name, e.g. "KindInvalidPort"
	value string // string value, e.g. "config.invalid_port"
}

// parseDeclaredKinds reads every internal/*/error.go and returns the Kind
// constants it declares. Reading source rather than importing the packages is
// what makes this catch a kind added later: Go cannot enumerate package-level
// constants at runtime, so an import-based check would only ever assert on the
// list someone remembered to update.
func parseDeclaredKinds() []declaredKind {
	files, err := filepath.Glob("../*/error.go")
	Expect(err).ShouldNot(HaveOccurred())
	Expect(files).ToNot(BeEmpty(), "no internal/*/error.go files found")

	var kinds []declaredKind

	for _, file := range files {
		pkg := filepath.Base(filepath.Dir(file))

		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		Expect(err).ShouldNot(HaveOccurred(), "parsing %s", file)

		for _, decl := range parsed.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}

			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok || len(value.Names) == 0 || len(value.Values) == 0 {
					continue
				}

				lit, ok := value.Values[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}

				unquoted, err := strconv.Unquote(lit.Value)
				Expect(err).ShouldNot(HaveOccurred())

				kinds = append(kinds, declaredKind{
					pkg:   pkg,
					name:  value.Names[0].Name,
					value: unquoted,
				})
			}
		}
	}

	return kinds
}

// errs.Kind is one shared type, so the compiler no longer stops a loader kind
// being compared against a config error the way per-package Kind types did.
// These specs are what replaces that: they hold the namespacing convention
// that keeps the values distinguishable.
var _ = Describe("declared kinds", func() {
	var kinds []declaredKind

	BeforeEach(func() {
		kinds = parseDeclaredKinds()
	})

	It("finds the kinds every package declares", func() {
		Expect(len(kinds)).To(BeNumerically(">=", 20))
	})

	It("gives every kind a unique value", func() {
		seen := map[string]string{}

		for _, k := range kinds {
			owner, dup := seen[k.value]
			Expect(dup).To(BeFalse(),
				"%s.%s duplicates the value of %s (%q) — errors.Is cannot tell "+
					"them apart", k.pkg, k.name, owner, k.value)
			seen[k.value] = k.pkg + "." + k.name
		}
	})

	It("namespaces every value by its owning package", func() {
		for _, k := range kinds {
			Expect(k.value).To(HavePrefix(k.pkg+"."),
				"%s.%s has value %q, which does not start with %q",
				k.pkg, k.name, k.value, k.pkg+".")
		}
	})
})
