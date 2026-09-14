package image

import "github.com/robgonnella/minienv/internal/errs"

const (
	ErrTemplateParse   errs.Kind = "image.template_parse"
	ErrTemplateExecute errs.Kind = "image.template_execute"
	ErrBuild           errs.Kind = "image.build"
)
