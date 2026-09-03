package loader

import "github.com/robgonnella/minienv/internal/errs"

// Failure modes raised by this package. Values are namespaced because errs.Kind
// is one shared type — see internal/errs.
const (
	KindProjectOptions    errs.Kind = "loader.project_options"
	KindProjectLoad       errs.Kind = "loader.project_load"
	KindNoExtension       errs.Kind = "loader.no_extension"
	KindExtensionDecode   errs.Kind = "loader.extension_decode"
	KindMultipleDeployers errs.Kind = "loader.multiple_deployers"
	KindNoActiveDeployer  errs.Kind = "loader.no_active_deployer"
)
