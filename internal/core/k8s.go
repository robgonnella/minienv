package core

import (
	"fmt"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/rs/zerolog/log"
	helmaction "helm.sh/helm/v3/pkg/action"
)

func k8sServicesDeploy(
	project *types.Project,
	extConfig *config.XMiniEnv,
	dryRun bool,
) error {
	if extConfig.K8s == nil {
		return nil
	}

	actionConfig, err := getHelmActionConfig(extConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize helm: %s", err)
	}

	if err := createNamespaceIfNotExists(
		actionConfig,
		extConfig.K8s.Namespace,
	); err != nil {
		return err
	}

	for _, svc := range project.Services {
		svcExt, err := config.NewXMiniEnvK8sService(extConfig, &svc)
		if err != nil {
			return err
		}

		if svcExt.Skip != nil && *svcExt.Skip {
			log.
				Warn().
				Str("service", svc.Name).
				Msg("detected skip: omitting service from deployment")
			continue
		}

		if err := upgradeOrInstallService(
			actionConfig,
			extConfig,
			svcExt,
			&svc,
			dryRun,
		); err != nil {
			return err
		}
	}

	return nil
}

func k8sServicesDestroy(
	project *types.Project,
	extConfig *config.XMiniEnv,
	dryRun bool,
) error {
	if extConfig.K8s == nil {
		return nil
	}

	actionConfig, err := getHelmActionConfig(extConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize helm: %s", err)
	}

	for name, svc := range project.Services {
		if err := uninstallChart(
			actionConfig,
			extConfig,
			&svc,
			dryRun,
		); err != nil {
			return fmt.Errorf("failed to destroy service %s: %s", name, err)
		}
	}

	return nil
}

func upgradeOrInstallService(
	actionConfig *helmaction.Configuration,
	mainExt *config.XMiniEnv,
	svcExt *config.XMiniEnvK8sService,
	svc *types.ServiceConfig,
	dryRun bool,
) error {
	exists := serviceReleaseExists(actionConfig, svc.Name)

	if exists {
		log.Info().Str("release", svc.Name).Msg("upgrading release")
		return upgradeChart(actionConfig, mainExt, svcExt, svc, dryRun)
	} else {
		log.Info().Str("release", svc.Name).Msg("installing release")
		return installChart(actionConfig, mainExt, svcExt, svc, dryRun)
	}
}
