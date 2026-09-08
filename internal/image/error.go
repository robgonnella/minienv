package image

import "github.com/robgonnella/minienv/internal/errs"

// Failure modes raised by this package. Values are namespaced because errs.Kind
// is one shared type — see internal/errs.
const (
	ErrTemplateParse   errs.Kind = "image.template_parse"
	ErrTemplateExecute errs.Kind = "image.template_execute"
	ErrBuild           errs.Kind = "image.build"
)
