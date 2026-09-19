// Package compose owns every compose-go call: loading a project, walking its
// includes, and the in-place rewrites that turn a locally loaded project into
// the one a remote host runs.
package compose

import (
	"bytes"
	"context"
	"fmt"
	"maps"

	composeloader "github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/errs"
)

type Project = types.Project
type Service = types.ServiceConfig

const bindMountType = "bind"

// ReloadWithNewName re-parses rather than renaming in place: compose derives
// volume, network and container names from the project name, and only a fresh
// load redoes that derivation.
func ReloadWithNewName(
	ctx context.Context,
	project Project,
	name string,
) (*Project, error) {
	normalizedName := composeloader.NormalizeProjectName(name)

	details, err := composeloader.LoadConfigFiles(
		ctx,
		project.ComposeFiles,
		project.WorkingDir,
		func(options *composeloader.Options) {
			options.SetProjectName(normalizedName, true)
		},
	)
	if err != nil {
		return nil, errs.Errorf(
			ErrReloadConfigFiles,
			"failed to reload config files: %w",
			err,
		)
	}

	// LoadConfigFiles leaves this nil, which resolves every ${VAR} to empty.
	// Cloned because the loader writes COMPOSE_PROJECT_NAME back into it.
	details.Environment = maps.Clone(project.Environment)

	newProject, err := composeloader.LoadWithContext(
		ctx,
		*details,
		func(options *composeloader.Options) {
			options.SetProjectName(normalizedName, true)
		},
	)
	if err != nil {
		return nil, errs.Errorf(
			ErrReloadProject,
			"failed reload project: %w",
			err,
		)
	}

	return newProject, nil
}

// RemoveServices deletes the named services along with every depends_on edge
// pointing at them, which compose would otherwise reject as dangling.
func RemoveServices(project *Project, names []string) {
	if len(names) == 0 {
		return
	}

	for _, name := range names {
		delete(project.Services, name)
	}

	for i, svc := range project.Services {
		for _, name := range names {
			delete(svc.DependsOn, name)
		}

		project.Services[i] = svc
	}
}

func ClearPortMappings(project *Project) {
	for i, svc := range project.Services {
		svc.Ports = nil
		project.Services[i] = svc
	}
}

func ClearBuildSettings(project *Project) {
	for i, svc := range project.Services {
		svc.Build = nil
		project.Services[i] = svc
	}
}

// ClearEnvAndLabelFiles drops paths that name the developer's machine. Compose
// has already merged what they contained into environment and labels.
func ClearEnvAndLabelFiles(project *Project) {
	for i, svc := range project.Services {
		svc.EnvFiles = nil
		svc.LabelFiles = nil
		project.Services[i] = svc
	}
}

func ClearEmptyCommandsAndEntryPoints(project *Project) {
	for i, svc := range project.Services {
		// An empty slice marshals as `command: []`, which compose reads as
		// "override to empty".
		if len(svc.Command) == 0 {
			svc.Command = nil
		}

		if len(svc.Entrypoint) == 0 {
			svc.Entrypoint = nil
		}

		project.Services[i] = svc
	}
}

func ClearServiceVolumes(project *Project) {
	for i, svc := range project.Services {
		vols := make([]types.ServiceVolumeConfig, 0, len(svc.Volumes))

		for _, vol := range svc.Volumes {
			// The host path behind a bind mount does not exist on the remote.
			if vol.Type == bindMountType {
				continue
			}

			vols = append(vols, vol)
		}

		svc.Volumes = vols
		project.Services[i] = svc
	}
}

func SetImages(
	project *Project,
	extensionMap map[string]config.ServiceImage,
) error {
	for i, svc := range project.Services {
		if svc.Image != "" {
			continue
		}

		svcExt, ok := extensionMap[svc.Name]
		if !ok {
			return errs.Errorf(
				ErrMissingService,
				"service is missing from extension map: %s",
				svc.Name,
			)
		}

		if svcExt.Repository == "" || svcExt.Tag == "" {
			return errs.Errorf(
				ErrMissingImage,
				"service does not have an image and none configured in extension: %s",
				svc.Name,
			)
		}

		svc.Image = fmt.Sprintf("%s:%s", svcExt.Repository, svcExt.Tag)
		project.Services[i] = svc
	}

	return nil
}

// InjectNgrokService turns configChecksum into a label; see
// config.NgrokConfigChecksumLabel.
func InjectNgrokService(
	project *Project,
	remoteConfigPath string,
	configChecksum string,
) {
	dependencies := types.DependsOnConfig{}

	for _, svc := range project.Services {
		dependencies[svc.Name] = types.ServiceDependency{
			Condition: "service_started",
		}
	}

	serviceName := "ngrok"
	service := Service{
		Name:  serviceName,
		Image: fmt.Sprintf("%s:%s", config.NgrokImageRepo, config.NgrokImageTag),
		Labels: types.Labels{
			config.NgrokConfigChecksumLabel: configChecksum,
		},
		// Overrides the image's entrypoint so Command runs.
		Entrypoint: []string{},
		Environment: types.MappingWithEquals{
			"NGROK_AUTHTOKEN": nil,
		},
		Command: []string{
			"ngrok",
			"start",
			"--all",
			"--log=stdout",
		},
		DependsOn: dependencies,
		Volumes: []types.ServiceVolumeConfig{
			{
				Type:   bindMountType,
				Source: remoteConfigPath,
				Target: fmt.Sprintf(
					"%s/%s",
					config.NgrokConfigVolMountPath,
					config.NgrokConfigKey,
				),
				ReadOnly: true,
			},
		},
	}
	project.Services[serviceName] = service
}

func BindVolume(
	project *Project,
	svcName string,
	hostPath string,
	containerPath string,
) {
	svc, ok := project.Services[svcName]
	if !ok {
		return
	}

	svc.Volumes = append(svc.Volumes, types.ServiceVolumeConfig{
		Type:   bindMountType,
		Source: hostPath,
		Target: containerPath,
	})

	project.Services[svcName] = svc
}

func Marshal(project *Project) ([]byte, error) {
	out, err := project.MarshalYAML()
	if err != nil {
		return nil, errs.Errorf(
			ErrMarshalProject,
			"failed to marshal compose project: %w",
			err,
		)
	}

	// The remote compose interpolates this file again, and it is already fully
	// resolved, so every literal $ has to survive that pass.
	return bytes.ReplaceAll(out, []byte("$"), []byte("$$")), nil
}
