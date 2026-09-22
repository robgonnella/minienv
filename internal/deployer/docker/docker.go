// Package docker deploys a rewritten compose project onto a remote host,
// reaching it over a transport rather than talking to a docker daemon locally.
package docker

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	composeloader "github.com/compose-spec/compose-go/v2/loader"
	"github.com/robgonnella/minienv/internal/compose"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/deployer"
	"github.com/robgonnella/minienv/internal/errs"
	"github.com/robgonnella/minienv/internal/git"
	"github.com/robgonnella/minienv/internal/image"
	"github.com/robgonnella/minienv/internal/publishing"
	"github.com/robgonnella/minienv/internal/resolver"
	"github.com/robgonnella/minienv/internal/transport"
	"github.com/rs/zerolog/log"
)

type remotePaths struct {
	projectDir  string
	composeFile string
	dotEnvFile  string
	ngrokConfig string
	copiesDir   string
}

const remoteCopiesDirName = "copies"

type remoteContent struct {
	compose []byte
	ngrok   []byte
	dotEnv  []byte
}

type Options struct {
	DockerExt      config.XMiniEnvDocker
	Source         compose.Source
	Transport      transport.Client
	ImageClient    image.Client
	GitClient      git.Client
	PublishClient  publishing.Client
	NgrokAuthToken string
	DryRun         bool
}

type Docker struct {
	dockerExt         config.XMiniEnvDocker
	source            compose.Source
	project           compose.Project
	services          map[string]resolver.DockerService
	servicesToPublish []config.NgrokEndpointConfig
	transport         transport.Client
	imageClient       image.Client
	gitClient         git.Client
	publishClient     publishing.Client
	ngrokAuthToken    string
	remotePaths       remotePaths
	dryRun            bool
}

func New(opts Options) *Docker {
	// Normalized once so the remote directory and the compose project name are
	// the same string
	opts.DockerExt.Namespace = composeloader.NormalizeProjectName(
		opts.DockerExt.Namespace,
	)

	projectDir := "~/.minienv/" + opts.DockerExt.Namespace
	composeFile := projectDir + "/compose.yml"
	dotEnvFile := projectDir + "/.env"
	ngrokConfig := projectDir + "/ngrok.yml"
	copiesDir := projectDir + "/" + remoteCopiesDirName

	return &Docker{
		dockerExt:      opts.DockerExt,
		source:         opts.Source,
		transport:      opts.Transport,
		imageClient:    opts.ImageClient,
		gitClient:      opts.GitClient,
		publishClient:  opts.PublishClient,
		ngrokAuthToken: opts.NgrokAuthToken,
		remotePaths: remotePaths{
			projectDir:  projectDir,
			dotEnvFile:  dotEnvFile,
			composeFile: composeFile,
			ngrokConfig: ngrokConfig,
			copiesDir:   copiesDir,
		},
		dryRun: opts.DryRun,
	}
}

func (d *Docker) String() string {
	return "Docker"
}

func (d *Docker) Init(ctx context.Context) error {
	if err := d.validateExtension(); err != nil {
		return err
	}

	project, err := compose.Load(ctx, d.source)
	if err != nil {
		return err
	}

	dirs, err := compose.LoadServiceDirs(ctx, project)
	if err != nil {
		return err
	}

	return d.initProject(ctx, *project, dirs)
}

func (d *Docker) Deploy(ctx context.Context) error {
	if err := d.validateExtension(); err != nil {
		return err
	}

	if err := d.validateInitialized(); err != nil {
		return err
	}

	if err := d.buildAndPushServiceImages(ctx); err != nil {
		return err
	}

	remoteContent, err := d.remoteContent(ctx)
	if err != nil {
		return err
	}

	if d.dryRun {
		d.logRemoteFilesDryRun()

		return nil
	}

	defer d.closeTransport()

	if err := d.writeRemoteFiles(ctx, remoteContent); err != nil {
		return err
	}

	if err := d.copyFiles(ctx); err != nil {
		return err
	}

	// --remove-orphans reaps the ngrok container left by a previous deploy that
	// published when this one does not.
	return d.transport.RunCommand(
		ctx,
		fmt.Sprintf(
			"cd %s && docker compose up -d --remove-orphans",
			d.remotePaths.projectDir,
		),
	)
}

func (d *Docker) Destroy(ctx context.Context) error {
	if err := d.validateExtension(); err != nil {
		return err
	}

	if err := d.validateInitialized(); err != nil {
		return err
	}

	if d.dryRun {
		log.Warn().Msg("dry-run: skipping destroy")
		return nil
	}

	defer d.closeTransport()

	// The directory holds the .env carrying the auth token, so removal is
	// sequenced rather than chained on down succeeding. Its status is kept.
	return d.transport.RunCommand(
		ctx,
		fmt.Sprintf(
			"cd %[1]s 2>/dev/null || exit 0; "+
				"docker compose down --volumes --remove-orphans; "+
				"status=$?; "+
				"rm -rf %[1]s; "+
				"exit $status",
			d.remotePaths.projectDir,
		),
	)
}

func (d *Docker) PublishedServiceUrls(
	ctx context.Context,
) (map[string]url.URL, error) {
	if len(d.servicesToPublish) == 0 {
		return map[string]url.URL{}, nil
	}

	return d.publishClient.ServiceUrls(
		ctx,
		deployer.PublishedNames(d.servicesToPublish),
	)
}

// Every extension is resolved before any is stored, so a bad one fails the
// whole project rather than one service midway.
func (d *Docker) initProject(
	ctx context.Context,
	project compose.Project,
	dirs compose.ServiceDirs,
) error {
	services := map[string]resolver.DockerService{}
	servicesToPublish := []config.NgrokEndpointConfig{}

	for _, svc := range project.Services {
		svcExt, err := resolver.NewDockerService(
			ctx,
			resolver.DockerServiceOptions{
				DockerExt:    d.dockerExt,
				Service:      svc,
				Dir:          dirs.Dir(svc.Name),
				GitClient:    d.gitClient,
				NgrokEnabled: d.ngrokAuthToken != "",
			},
		)
		if err != nil {
			return err
		}

		services[svc.Name] = *svcExt

		// Port is zero unless ngrok is configured and usable.
		if svcExt.Ngrok.Port != 0 {
			// A skipped service is absent from the remote project, so there is
			// nothing for the endpoint's upstream to route to.
			switch {
			case svcExt.Skip:
				log.
					Warn().
					Str("service", svc.Name).
					Msg("detected skip: not publishing an ngrok endpoint")
			default:
				servicesToPublish = append(servicesToPublish, config.NgrokEndpointConfig{
					Namespace:     d.dockerExt.Namespace,
					EndpointName:  fmt.Sprintf("%s-%s", d.dockerExt.Namespace, svc.Name),
					ServiceName:   svc.Name,
					URL:           svcExt.Ngrok.URL,
					Port:          svcExt.Ngrok.Port,
					TrafficPolicy: svcExt.Ngrok.TrafficPolicy,
				})
			}
		}
	}

	// project.Services is a map. An unstable order rewrites ngrok.yml on an
	// unchanged deploy, replacing the agent and reassigning unreserved URLs.
	slices.SortFunc(servicesToPublish, func(a, b config.NgrokEndpointConfig) int {
		return strings.Compare(a.ServiceName, b.ServiceName)
	})

	d.services = services
	d.servicesToPublish = servicesToPublish
	d.project = project

	return nil
}

// New normalizes first, so this also catches a namespace that reduces to
// nothing — which would aim Destroy's removal at ~/.minienv itself.
func (d *Docker) validateExtension() error {
	if d.dockerExt.Namespace == "" {
		return errs.Errorf(
			ErrInvalidExtension,
			"missing or unusable required field in docker extension config: "+
				"[namespace]. must contain at least one of [a-z0-9_-]",
		)
	}

	return nil
}

// Without Init, Deploy would write an empty project rather than failing.
func (d *Docker) validateInitialized() error {
	if d.services == nil {
		return errs.Errorf(
			ErrNotInitialized,
			"deployer was not initialized: Init must run before this call",
		)
	}

	return nil
}

func (d *Docker) closeTransport() {
	if err := d.transport.Close(); err != nil {
		log.Warn().Err(err).Msg("failed to close transport")
	}
}

func (d *Docker) buildAndPushServiceImages(ctx context.Context) error {
	dockerServices := []image.ServiceProperties{}

	for name, svc := range d.services {
		if svc.Skip {
			log.
				Warn().
				Str("service", name).
				Msg("detected skip: omitting service from deployment")

			continue
		}

		if svc.Compose.Build != nil {
			dockerServices = append(dockerServices, image.NewServiceProperties(
				svc.Compose,
				svc.Image,
			))
		}
	}

	if len(dockerServices) > 0 {
		return d.imageClient.BuildAndPush(ctx, dockerServices)
	}

	return nil
}

func (d *Docker) writeRemoteFiles(
	ctx context.Context,
	content *remoteContent,
) error {
	if err := d.transport.CreateFile(
		ctx,
		d.remotePaths.composeFile,
		content.compose,
	); err != nil {
		return err
	}

	// --remove-orphans reaps the agent's container but not the .env holding the
	// auth token a previous publishing deploy left beside it.
	if len(d.servicesToPublish) == 0 {
		return d.transport.RunCommand(
			ctx,
			fmt.Sprintf(
				"rm -f %s %s",
				d.remotePaths.dotEnvFile,
				d.remotePaths.ngrokConfig,
			),
		)
	}

	if err := d.transport.CreateFile(
		ctx,
		d.remotePaths.dotEnvFile,
		content.dotEnv,
	); err != nil {
		return err
	}

	return d.transport.CreateFile(
		ctx,
		d.remotePaths.ngrokConfig,
		content.ngrok,
	)
}

// Paths only: the rendered compose file is fully interpolated by this point, so
// its body carries whatever the project's own ${VAR}s resolved to.
func (d *Docker) logRemoteFilesDryRun() {
	log.Warn().Msg("dry-run: skipping deploy")
	log.
		Warn().
		Msgf("would have created compose config: %s", d.remotePaths.composeFile)

	for _, name := range d.copyableServices() {
		svc := d.services[name]

		for _, c := range svc.Copy {
			log.
				Warn().
				Str("service", name).
				Msgf(
					"would have copied %s to %s",
					svc.CopyLocalPath(c),
					d.copyRemotePath(name, c),
				)
		}
	}

	if len(d.servicesToPublish) == 0 {
		return
	}

	log.
		Warn().
		Msgf("would have created ngrok config: %s", d.remotePaths.ngrokConfig)
	log.
		Warn().
		Msgf("would have created env file: %s", d.remotePaths.dotEnvFile)
}

func (d *Docker) remoteContent(ctx context.Context) (*remoteContent, error) {
	// Rendered before the rewrite: the injected service labels these bytes.
	var ngrokConfigContent []byte

	var err error

	if len(d.servicesToPublish) > 0 {
		ngrokConfigContent, err = d.ngrokConfigContent()
		if err != nil {
			return nil, err
		}
	}

	newComposeContent, err := d.modifyComposeContent(ctx, ngrokConfigContent)
	if err != nil {
		return nil, err
	}

	return &remoteContent{
		compose: newComposeContent,
		ngrok:   ngrokConfigContent,
		dotEnv:  d.remoteDotEnvContent(),
	}, nil
}

func (d *Docker) skippedServices() []string {
	names := []string{}

	for name, svc := range d.services {
		if svc.Skip {
			names = append(names, name)
		}
	}

	return names
}

func (d *Docker) modifyComposeContent(
	ctx context.Context,
	ngrokConfigContent []byte,
) ([]byte, error) {
	project, err := compose.ReloadWithNewName(
		ctx,
		d.project,
		d.dockerExt.Namespace,
	)
	if err != nil {
		return nil, err
	}

	// Removal comes first: InjectNgrokService builds depends_on from whatever
	// services remain, and would otherwise re-add an edge to a skipped one.
	compose.RemoveServices(project, d.skippedServices())
	compose.ClearBuildSettings(project)
	compose.ClearPortMappings(project)
	compose.ClearServiceVolumes(project)
	compose.ClearEnvAndLabelFiles(project)
	compose.ClearEmptyCommandsAndEntryPoints(project)

	d.bindCopies(project)

	if len(d.servicesToPublish) > 0 {
		compose.InjectNgrokService(
			project,
			d.remotePaths.ngrokConfig,
			ngrokConfigChecksum(ngrokConfigContent, d.ngrokAuthToken),
		)
	}

	extensionMap := map[string]config.ServiceImage{}
	for k, v := range d.services {
		extensionMap[k] = v.Image
	}

	if err := compose.SetImages(project, extensionMap); err != nil {
		return nil, err
	}

	fileContent, err := compose.Marshal(project)
	if err != nil {
		return nil, errs.Errorf(
			ErrMarshalProject,
			"failed to generate modified compose.yml for remote environment: %w",
			err,
		)
	}

	return fileContent, nil
}

func (d *Docker) remoteDotEnvContent() []byte {
	return []byte("NGROK_AUTHTOKEN=" + d.ngrokAuthToken)
}

func (d *Docker) ngrokConfigContent() ([]byte, error) {
	tmpl, err := template.
		New("tmpl").
		Funcs(sprig.FuncMap()).
		Parse(ngrokConfigTmpl)
	if err != nil {
		return nil, errs.Errorf(
			ErrTemplateParse,
			"failed to parse ngrok config template: %w",
			err,
		)
	}

	out := bytes.NewBuffer([]byte{})

	err = tmpl.Execute(out, d.servicesToPublish)
	if err != nil {
		return nil, errs.Errorf(
			ErrTemplateExecute,
			"failed to execute ngrok config template: %w",
			err,
		)
	}

	return out.Bytes(), nil
}

// Per service: two services may declare the same hostPath from different
// directories.
func (d *Docker) copyRemotePath(
	name string,
	c config.XMiniEnvDockerCopy,
) string {
	return d.remotePaths.copiesDir + "/" + name + "/" +
		filepath.ToSlash(c.HostPath)
}

// Sorted: an unstable order rewrites compose.yml on an unchanged deploy.
func (d *Docker) copyableServices() []string {
	names := []string{}

	for name, svc := range d.services {
		// Nothing on the remote would mount what a skipped service declares.
		if svc.Skip {
			continue
		}

		if len(svc.Copy) > 0 {
			names = append(names, name)
		}
	}

	slices.Sort(names)

	return names
}

func (d *Docker) bindCopies(project *compose.Project) {
	for _, name := range d.copyableServices() {
		for _, c := range d.services[name].Copy {
			compose.BindVolume(
				project,
				name,
				d.copyRemotePath(name, c),
				c.ContainerPath,
			)
		}
	}
}

// Replaced rather than merged, so a path dropped from the config leaves
// nothing behind.
func (d *Docker) copyFiles(ctx context.Context) error {
	if err := d.transport.RunCommand(
		ctx,
		"rm -rf "+d.remotePaths.copiesDir,
	); err != nil {
		return err
	}

	for _, name := range d.copyableServices() {
		svc := d.services[name]

		for _, c := range svc.Copy {
			if err := d.transport.CopyPath(
				ctx,
				svc.CopyLocalPath(c),
				d.copyRemotePath(name, c),
			); err != nil {
				return err
			}
		}
	}

	return nil
}
