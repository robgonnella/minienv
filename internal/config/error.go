package config

import "github.com/robgonnella/minienv/internal/errs"

// Failure modes raised by this package. Values are namespaced because errs.Kind
// is one shared type — see internal/errs.
const (
	KindK8sNotConfigured       errs.Kind = "config.k8s_not_configured"
	KindExtensionDecode        errs.Kind = "config.extension_decode"
	KindValuesDecode           errs.Kind = "config.values_decode"
	KindImageRepositoryMissing errs.Kind = "config.image_repository_missing"
	KindImageTagMissing        errs.Kind = "config.image_tag_missing"
	KindGitShortSha            errs.Kind = "config.git_short_sha"
	KindInvalidPort            errs.Kind = "config.invalid_port"
	KindInvalidPublishedPort   errs.Kind = "config.invalid_published_port"
	KindNgrokPortMismatch      errs.Kind = "config.ngrok_port_mismatch"
	KindInvalidDeploymentType  errs.Kind = "config.invalid_deployment_type"
)
