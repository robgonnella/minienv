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
	kindA errs.Kind = "test.a"
	kindB errs.Kind = "test.b"
)

var _ = Describe("Error", func() {
	It("carries the kind it was built with", func() {
		err := errs.Errorf(kindA, "boom")

		Expect(err.Kind()).To(Equal(kindA))
		Expect(err).To(MatchError(kindA))
		Expect(err).ToNot(MatchError(kindB))
	})

	It("unwraps a cause captured with %w", func() {
		cause := errors.New("cause")
		err := errs.Errorf(kindA, "wrapped: %w", cause)

		Expect(errors.Unwrap(err)).To(BeIdenticalTo(cause))
		Expect(err).To(MatchError(cause))
	})

	It("has no cause when nothing was wrapped", func() {
		Expect(errors.Unwrap(errs.Errorf(kindA, "x"))).To(BeNil())
	})

	It("matches a kind through a wrap chain", func() {
		// config wraps git's failure for the +git tag convention, so a kind
		// has to stay matchable from more than one level down.
		inner := errs.Errorf(kindA, "inner")
		outer := errs.Errorf(kindB, "outer: %w", inner)

		Expect(outer).To(MatchError(kindB))
		Expect(outer).To(MatchError(kindA))
	})

	It("matches a kind through an errors.Join tree", func() {
		// resolveServiceImage accumulates its validation failures this way.
		joined := errors.Join(errors.New("unrelated"), errs.Errorf(kindA, "a"))

		Expect(joined).To(MatchError(kindA))
		Expect(joined).ToNot(MatchError(kindB))
	})
})

var _ = Describe("Kind", func() {
	It("reads as its own string so it can double as a sentinel", func() {
		Expect(kindA.Error()).To(Equal("test.a"))
	})
})
