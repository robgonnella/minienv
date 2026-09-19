package compose

import "github.com/robgonnella/minienv/internal/errs"

const (
	ErrProjectOptions    errs.Kind = "compose.project_options"
	ErrProjectLoad       errs.Kind = "compose.project_load"
	ErrIncludeModel      errs.Kind = "compose.include_model"
	ErrIncludeDecode     errs.Kind = "compose.include_decode"
	ErrIncludeCycle      errs.Kind = "compose.include_cycle"
	ErrServiceDirMissing errs.Kind = "compose.service_dir_missing"
	ErrRawModel          errs.Kind = "compose.raw_model"
	ErrReloadConfigFiles errs.Kind = "compose.reload_config_files"
	ErrReloadProject     errs.Kind = "compose.reload_project"
	ErrMissingService    errs.Kind = "compose.missing_service"
	ErrMissingImage      errs.Kind = "compose.missing_image"
	ErrMarshalProject    errs.Kind = "compose.marshal_project"
)
