package compose

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/v2/dotenv"
	composeloader "github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/robgonnella/minienv/internal/errs"
)

type ServiceDirs map[string]string

func (s ServiceDirs) Dir(name string) string {
	return s[name]
}

type RawService struct {
	Environment any
	Extensions  map[string]any
}

type RawServices map[string]RawService

func (r RawServices) Get(name string) RawService {
	return r[name]
}

const extensionPrefix = "x-"

// LoadServiceDirs recovers which compose file declared each service: compose
// merges included files and drops the directory context. This makes it
// available again.
func LoadServiceDirs(
	ctx context.Context,
	project *Project,
) (ServiceDirs, error) {
	walk, err := walkServices(ctx, project, false)
	if err != nil {
		return nil, err
	}

	return walk.dirs, nil
}

func LoadRawServices(
	ctx context.Context,
	project *Project,
) (RawServices, error) {
	walk, err := walkServices(ctx, project, true)
	if err != nil {
		return nil, err
	}

	return walk.raw, nil
}

type serviceWalk struct {
	projectName string
	withRaw     bool
	visited     map[string]bool
	dirs        ServiceDirs
	raw         RawServices
}

func walkServices(
	ctx context.Context,
	project *Project,
	withRaw bool,
) (*serviceWalk, error) {
	walk := &serviceWalk{
		projectName: project.Name,
		withRaw:     withRaw,
		visited:     map[string]bool{},
		dirs:        ServiceDirs{},
		raw:         RawServices{},
	}

	if err := walk.visit(
		ctx,
		project.ComposeFiles,
		project.WorkingDir,
		project.Environment,
		nil,
	); err != nil {
		return nil, err
	}

	for name := range project.Services {
		if _, ok := walk.dirs[name]; !ok {
			return nil, errs.Errorf(
				ErrServiceDirMissing,
				"failed to find the compose file declaring service %s",
				name,
			)
		}
	}

	return walk, nil
}

func (w *serviceWalk) visit(
	ctx context.Context,
	files []string,
	dir string,
	env types.Mapping,
	envFiles []string,
) error {
	if len(files) == 0 {
		return errs.Errorf(ErrIncludeDecode, "include declares no path")
	}

	main, err := filepath.Abs(files[0])
	if err != nil {
		return errs.Errorf(
			ErrIncludeModel,
			"failed to resolve compose file %s: %w",
			files[0],
			err,
		)
	}

	if w.visited[main] {
		return errs.Errorf(ErrIncludeCycle, "include cycle at %s", main)
	}

	w.visited[main] = true

	model, err := w.loadModel(ctx, files, dir, env, envFiles)
	if err != nil {
		return err
	}

	w.claimDirs(model, dir)

	if err := w.claimRaw(ctx, files, dir); err != nil {
		return err
	}

	includes, err := includeConfigs(model["include"])
	if err != nil {
		return err
	}

	for _, inc := range includes {
		resolved, err := resolveInclude(dir, inc)
		if err != nil {
			return err
		}

		if err := w.visit(
			ctx,
			resolved.Path,
			resolved.ProjectDirectory,
			env,
			resolved.EnvFile,
		); err != nil {
			return err
		}
	}

	return nil
}

// The includer is visited before its includes, and compose lets the includer
// override on conflict, so the first claim wins.
func (w *serviceWalk) claimDirs(model map[string]any, dir string) {
	services, ok := model["services"].(map[string]any)
	if !ok {
		return
	}

	for name := range services {
		if _, claimed := w.dirs[name]; !claimed {
			w.dirs[name] = dir
		}
	}
}

func (w *serviceWalk) claimRaw(
	ctx context.Context,
	files []string,
	dir string,
) error {
	if !w.withRaw {
		return nil
	}

	model, err := w.loadRawModel(ctx, files, dir)
	if err != nil {
		return err
	}

	services, ok := model["services"].(map[string]any)
	if !ok {
		return nil
	}

	for name, svc := range services {
		if _, claimed := w.raw[name]; claimed {
			continue
		}

		body, ok := svc.(map[string]any)
		if !ok {
			continue
		}

		w.raw[name] = RawService{
			Environment: body["environment"],
			Extensions:  extensionsOf(body),
		}
	}

	return nil
}

func extensionsOf(body map[string]any) map[string]any {
	extensions := map[string]any{}

	for key, value := range body {
		if strings.HasPrefix(key, extensionPrefix) {
			extensions[key] = value
		}
	}

	return extensions
}

// compose-go leaves include paths relative in the model, so they are joined
// here the way its ApplyInclude joins them, default .env included.
func resolveInclude(
	dir string,
	inc types.IncludeConfig,
) (types.IncludeConfig, error) {
	if len(inc.Path) == 0 {
		return inc, errs.Errorf(ErrIncludeDecode, "include declares no path")
	}

	resolved := types.IncludeConfig{
		Path:             make(types.StringList, 0, len(inc.Path)),
		ProjectDirectory: "",
		EnvFile:          make(types.StringList, 0, len(inc.EnvFile)),
	}

	for _, p := range inc.Path {
		resolved.Path = append(resolved.Path, absoluteTo(dir, p))
	}

	resolved.ProjectDirectory = filepath.Dir(resolved.Path[0])
	if inc.ProjectDirectory != "" {
		resolved.ProjectDirectory = absoluteTo(dir, inc.ProjectDirectory)
	}

	for _, f := range inc.EnvFile {
		resolved.EnvFile = append(resolved.EnvFile, absoluteTo(dir, f))
	}

	if len(resolved.EnvFile) == 0 {
		dotEnv := filepath.Join(resolved.ProjectDirectory, ".env")
		if s, err := os.Stat(dotEnv); err == nil && !s.IsDir() {
			resolved.EnvFile = types.StringList{dotEnv}
		}
	}

	return resolved, nil
}

func absoluteTo(dir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}

	return filepath.Join(dir, path)
}

func (w *serviceWalk) loadModel(
	ctx context.Context,
	files []string,
	dir string,
	env types.Mapping,
	envFiles []string,
) (map[string]any, error) {
	fromFiles, err := dotenv.GetEnvFromFile(env, envFiles)
	if err != nil {
		return nil, errs.Errorf(
			ErrIncludeModel,
			"failed to load env files for compose file %s: %w",
			files[0],
			err,
		)
	}

	// Called directly: ProjectOptions.LoadModel drops the environment, so
	// nothing would interpolate.
	details := types.ConfigDetails{
		WorkingDir:  dir,
		ConfigFiles: types.ToConfigFiles(files),
		Environment: env.Clone().Merge(fromFiles),
	}

	// SkipInclude keeps the include key in the model. The other two are what
	// compose-go itself skips for a file that may reference its includer.
	model, err := composeloader.LoadModelWithContext(
		ctx,
		details,
		func(o *composeloader.Options) {
			o.SetProjectName(w.projectName, true)
			o.SkipInclude = true
			o.SkipNormalization = true
			o.SkipConsistencyCheck = true
		},
	)
	if err != nil {
		return nil, errs.Errorf(
			ErrIncludeModel,
			"failed to load compose file %s: %w",
			files[0],
			err,
		)
	}

	return model, nil
}

func (w *serviceWalk) loadRawModel(
	ctx context.Context,
	files []string,
	dir string,
) (map[string]any, error) {
	details := types.ConfigDetails{
		WorkingDir:  dir,
		ConfigFiles: types.ToConfigFiles(files),
	}

	model, err := composeloader.LoadModelWithContext(
		ctx,
		details,
		func(o *composeloader.Options) {
			o.SetProjectName(w.projectName, true)
			o.SkipInclude = true
			o.SkipNormalization = true
			o.SkipConsistencyCheck = true
			o.SkipInterpolation = true
			o.SkipValidation = true
		},
	)
	if err != nil {
		return nil, errs.Errorf(
			ErrRawModel,
			"failed to load uninterpolated compose file %s: %w",
			files[0],
			err,
		)
	}

	return model, nil
}

func includeConfigs(source any) ([]types.IncludeConfig, error) {
	if source == nil {
		return nil, nil
	}

	entries, ok := source.([]any)
	if !ok {
		return nil, errs.Errorf(
			ErrIncludeDecode,
			"include must be a list, got %T",
			source,
		)
	}

	// Same shorthand expansion as compose-go's unexported loadIncludeConfig.
	for i, entry := range entries {
		if path, ok := entry.(string); ok {
			entries[i] = map[string]any{"path": path}
		}
	}

	var configs []types.IncludeConfig
	if err := composeloader.Transform(entries, &configs); err != nil {
		return nil, errs.Errorf(
			ErrIncludeDecode,
			"failed to decode include: %w",
			err,
		)
	}

	return configs, nil
}
