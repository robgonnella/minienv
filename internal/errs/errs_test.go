package errs_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/errs"
)

// Kinds local to this suite. Using real package kinds here would couple the
// mechanism's specs to whatever failure modes happen to exist today.
const (
	errKindA errs.Kind = "test.a"
	errKindB errs.Kind = "test.b"
)

var _ = Describe("Error", func() {
	It("carries the kind it was built with", func() {
		err := errs.Errorf(errKindA, "boom")

		Expect(err.Kind()).To(Equal(errKindA))
		Expect(err).To(MatchError(errKindA))
		Expect(err).ToNot(MatchError(errKindB))
	})

	It("unwraps a cause captured with %w", func() {
		cause := errors.New("cause")
		err := errs.Errorf(errKindA, "wrapped: %w", cause)

		Expect(errors.Unwrap(err)).To(BeIdenticalTo(cause))
		Expect(err).To(MatchError(cause))
	})

	It("has no cause when nothing was wrapped", func() {
		Expect(errors.Unwrap(errs.Errorf(errKindA, "x"))).To(Succeed())
	})

	It("matches a kind through a wrap chain", func() {
		// config wraps git's failure for the +git tag convention, so a kind
		// has to stay matchable from more than one level down.
		inner := errs.Errorf(errKindA, "inner")
		outer := errs.Errorf(errKindB, "outer: %w", inner)

		Expect(outer).To(MatchError(errKindB))
		Expect(outer).To(MatchError(errKindA))
	})

	It("matches a kind through an errors.Join tree", func() {
		// resolveServiceImage accumulates its validation failures this way.
		joined := errors.Join(errors.New("unrelated"), errs.Errorf(errKindA, "a"))

		Expect(joined).To(MatchError(errKindA))
		Expect(joined).ToNot(MatchError(errKindB))
	})
})

var _ = Describe("Kind", func() {
	It("reads as its own string so it can double as a sentinel", func() {
		Expect(errKindA.Error()).To(Equal("test.a"))
	})
})
