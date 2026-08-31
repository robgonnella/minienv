package core

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/go-viper/mapstructure/v2"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/rs/zerolog/log"
	"helm.sh/helm/v3/pkg/action"
	helmaction "helm.sh/helm/v3/pkg/action"
	helmcli "helm.sh/helm/v3/pkg/cli"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func k8sServicesDeploy(
	project *types.Project,
	extConfig *config.XMiniEnv,
	dryRun bool,
) error {
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
		if extConfig.K8s != nil {
			svcExt, ok := svc.Extensions[config.SERVICE_K8S_EXTENSION]
			if !ok {
				svcExt = map[string]any{}
			}

			log.Info().Str("service", svc.Name).Msg("processing service")

			var svcExtConfig config.XMiniEnvK8sService
			if err := mapstructure.Decode(svcExt, &svcExtConfig); err != nil {
				return fmt.Errorf(
					"failed to parse x-minienv-k8s-service extension: %s",
					err,
				)
			}

			if svcExtConfig.Skip != nil && *svcExtConfig.Skip {
				continue
			}

			if err := upgradeOrInstallService(
				actionConfig,
				extConfig,
				&svcExtConfig,
				&svc,
				dryRun,
			); err != nil {
				return err
			}
		}
	}

	return nil
}

func k8sServicesDestroy(
	project *types.Project,
	extConfig *config.XMiniEnv,
	dryRun bool,
) error {
	actionConfig, err := getHelmActionConfig(extConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize helm: %s", err)
	}

	for name, svc := range project.Services {
		if extConfig.K8s != nil {
			svcExt, ok := svc.Extensions[config.SERVICE_K8S_EXTENSION]
			if !ok {
				svcExt = map[string]any{}
			}

			log.Info().Str("service", svc.Name).Msg("processing service")

			var svcExtConfig config.XMiniEnvK8sService
			if err := mapstructure.Decode(svcExt, &svcExtConfig); err != nil {
				return fmt.Errorf(
					"failed to parse x-minienv-k8s-service extension: %s",
					err,
				)
			}

			if svcExtConfig.Skip != nil && *svcExtConfig.Skip {
				continue
			}

			if err := destroyService(
				actionConfig,
				extConfig,
				&svc,
				dryRun,
			); err != nil {
				return fmt.Errorf("failed to destroy service %s: %s", name, err)
			}
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
	exists, err := serviceReleaseExists(actionConfig, svc.Name)
	if err != nil {
		return err
	}

	if exists {
		log.Info().Str("release", svc.Name).Msg("upgrading release")
		return upgradeChart(actionConfig, mainExt, svcExt, svc, dryRun)
	} else {
		log.Info().Str("release", svc.Name).Msg("installing release")
		return installChart(actionConfig, mainExt, svcExt, svc, dryRun)
	}
}

func installChart(
	actionConfig *helmaction.Configuration,
	mainExt *config.XMiniEnv,
	svcExt *config.XMiniEnvK8sService,
	svc *types.ServiceConfig,
	dryRun bool,
) error {
	values, err := svcExt.Values.Resolve(svc)
	if err != nil {
		return err
	}

	defaultTimeout := "2m"
	timeout := svcExt.DeploymentTimeout
	if timeout == nil {
		timeout = &defaultTimeout
	}

	parsedTimeout, err := time.ParseDuration(*timeout)
	if err != nil {
		return fmt.Errorf("invalid deploymentTimeout configuration: %s", err)
	}

	client := helmaction.NewInstall(actionConfig)
	client.ReleaseName = svc.Name
	client.Namespace = mainExt.K8s.Namespace
	client.CreateNamespace = false
	client.Wait = true
	client.Atomic = true
	client.Wait = true
	client.DryRun = dryRun
	client.Timeout = parsedTimeout

	chart, err := loadChart(svc.Name)
	if err != nil {
		return fmt.Errorf("failed to load in-memory chart: %s", err)
	}

	if _, err := client.Run(chart, values); err != nil {
		return fmt.Errorf("failed to install service chart %s: %s", svc.Name, err)
	}

	log.
		Info().
		Str("context", mainExt.K8s.Context).
		Str("namespace", mainExt.K8s.Namespace).
		Str("service", svc.Name).
		Msg("successfully installed service chart")

	return nil
}

func upgradeChart(
	actionConfig *helmaction.Configuration,
	mainExt *config.XMiniEnv,
	svcExt *config.XMiniEnvK8sService,
	svc *types.ServiceConfig,
	dryRun bool,
) error {
	values, err := svcExt.Values.Resolve(svc)
	if err != nil {
		return err
	}

	defaultTimeout := "2m"
	timeout := svcExt.DeploymentTimeout
	if timeout == nil {
		timeout = &defaultTimeout
	}

	parsedTimeout, err := time.ParseDuration(*timeout)
	if err != nil {
		return fmt.Errorf("invalid deploymentTimeout configuration: %s", err)
	}

	client := helmaction.NewUpgrade(actionConfig)
	client.Namespace = mainExt.K8s.Namespace
	client.Atomic = true
	client.Wait = true
	client.CleanupOnFail = true
	client.DryRun = dryRun
	client.Timeout = parsedTimeout

	chart, err := loadChart(svc.Name)
	if err != nil {
		return fmt.Errorf("failed to load in-memory chart: %s", err)
	}

	if _, err := client.Run(svc.Name, chart, values); err != nil {
		return fmt.Errorf("failed to upgrade service chart %s: %s", svc.Name, err)
	}

	log.
		Info().
		Str("context", mainExt.K8s.Context).
		Str("namespace", mainExt.K8s.Namespace).
		Str("service", svc.Name).
		Msg("successfully upgraded service chart")

	return nil
}

func destroyService(
	actionConfig *helmaction.Configuration,
	mainExt *config.XMiniEnv,
	svc *types.ServiceConfig,
	dryRun bool,
) error {
	client := helmaction.NewUninstall(actionConfig)
	client.Wait = true
	client.IgnoreNotFound = true
	client.DryRun = dryRun

	response, err := client.Run(svc.Name)
	if err != nil {
		return err
	}

	log.
		Info().
		Str("context", mainExt.K8s.Context).
		Str("service", response.Release.Name).
		Str("namespace", response.Release.Namespace).
		Msg("successfully uninstalled service")

	return nil
}

func getHelmActionConfig(
	extConfig *config.XMiniEnv,
) (*helmaction.Configuration, error) {
	settings := helmcli.New()
	settings.SetNamespace(extConfig.K8s.Namespace)
	settings.KubeContext = extConfig.K8s.Context

	actionConfig := new(helmaction.Configuration)

	if err := actionConfig.Init(
		settings.RESTClientGetter(),
		settings.Namespace(),
		os.Getenv("HELM_DRIVER"),
		log.Printf,
	); err != nil {
		return nil, err
	}

	return actionConfig, nil
}

func createNamespaceIfNotExists(
	actionConfig *action.Configuration,
	namespace string,
) error {
	clientset, err := actionConfig.KubernetesClientSet()
	if err != nil {
		return err
	}

	create := false

	_, err = clientset.
		CoreV1().
		Namespaces().
		Get(context.TODO(), namespace, metav1.GetOptions{})

	if err != nil && errors.IsNotFound(err) {
		create = true
	} else if err != nil {
		return err
	}

	if !create {
		return nil
	}

	_, err = clientset.
		CoreV1().
		Namespaces().
		Create(
			context.TODO(),
			&v1.Namespace{Name: namespace},
			metav1.CreateOptions{},
		)

	return err
}

func serviceReleaseExists(
	actionConfig *action.Configuration,
	name string,
) (bool, error) {
	client := helmaction.NewGet(actionConfig)
	release, err := client.Run(name)

	if err != nil && err.Error() == "release: not found" {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return release != nil, nil
}
