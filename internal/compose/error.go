package compose

import "github.com/robgonnella/minienv/internal/errs"

// Failure modes raised by this package. Values are namespaced because errs.Kind
// is one shared type — see internal/errs.
const (
	ErrReloadConfigFiles errs.Kind = "compose.reload_config_files"
	ErrReloadProject     errs.Kind = "compose.reload_project"
	ErrMissingService    errs.Kind = "compose.missing_service"
	ErrMissingImage      errs.Kind = "compose.missing_image"
	ErrMarshalProject    errs.Kind = "compose.marshal_project"
)
