package docker

import "github.com/robgonnella/minienv/internal/errs"

const (
	ErrInvalidExtension errs.Kind = "docker.invalid_extension"
	ErrNotInitialized   errs.Kind = "docker.not_initialized"
	ErrMarshalProject   errs.Kind = "docker.marshal_project"
	ErrTemplateParse    errs.Kind = "docker.template_parse"
	ErrTemplateExecute  errs.Kind = "docker.template_execute"
)
