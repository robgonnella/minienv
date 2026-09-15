package compose_test

import (
	"context"
	"os"
	"path/filepath"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/robgonnella/minienv/internal/compose"
	"github.com/robgonnella/minienv/internal/config"
	"gopkg.in/yaml.v3"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// newProject returns a fresh fixture per spec: every function under test
// mutates the project in place.
func newProject() *config.ComposeProject {
	return &config.ComposeProject{
		Name: "demo",
		Services: types.Services{
			"api": {
				Name:  "api",
				Build: &types.BuildConfig{Context: ".", Dockerfile: "Dockerfile"},
				Ports: []types.ServicePortConfig{
					{Target: 8080, Published: "8081", Protocol: "tcp"},
				},
				Volumes: []types.ServiceVolumeConfig{
					{Type: "bind", Source: "./conf", Target: "/etc/conf"},
					{Type: "volume", Source: "data", Target: "/var/lib/data"},
				},
				Command:    types.ShellCommand{},
				Entrypoint: types.ShellCommand{},
				// Absolute because compose resolves them at load; their
				// contents are already in Environment and Labels.
				EnvFiles:    []types.EnvFile{{Path: "/home/dev/demo/.env.api"}},
				LabelFiles:  []string{"/home/dev/demo/labels.env"},
				Environment: types.MappingWithEquals{"FROM_ENV_FILE": nil},
				Labels:      types.Labels{"from.label.file": "yes"},
				DependsOn: types.DependsOnConfig{
					"job": {Condition: "service_completed_successfully"},
					"db":  {Condition: "service_healthy"},
				},
			},
			"job": {
				Name:  "job",
				Image: "alpine:3.21",
				DependsOn: types.DependsOnConfig{
					"db": {Condition: "service_healthy"},
				},
			},
			"db": {
				Name:  "db",
				Image: "postgres:15",
			},
		},
	}
}

var _ = Describe("Compose", func() {
	Describe("ReloadWithNewName", func() {
		const composeFile = `
name: original
services:
  api:
    image: alpine:3.21
    environment:
      GREETING: ${GREETING_VAR}
volumes:
  data:
`

		// Asserted together: either half alone passes while the other is broken.
		It("renames the project without losing interpolation", func() {
			dir := GinkgoT().TempDir()
			file := filepath.Join(dir, "compose.yml")

			Expect(os.WriteFile(file, []byte(composeFile), 0o600)).To(Succeed())

			project := config.ComposeProject{
				Name:         "original",
				WorkingDir:   dir,
				ComposeFiles: []string{file},
				Environment:  types.Mapping{"GREETING_VAR": "hello"},
			}

			reloaded, err := compose.ReloadWithNewName(
				context.Background(),
				project,
				"my-namespace",
			)

			Expect(err).ToNot(HaveOccurred())
			Expect(reloaded.Name).To(Equal("my-namespace"))
			Expect(reloaded.Volumes["data"].Name).
				To(HavePrefix("my-namespace"), "volume name not re-derived")
			Expect(reloaded.Services["api"].Environment).
				To(HaveKeyWithValue("GREETING", new("hello")))
		})

		It("resolves a variable the caller's environment defined", func() {
			dir := GinkgoT().TempDir()
			file := filepath.Join(dir, "compose.yml")

			Expect(os.WriteFile(file, []byte(`
name: original
services:
  api:
    image: reg/api:${TAG_VAR}
`), 0o600)).To(Succeed())

			project := config.ComposeProject{
				Name:         "original",
				WorkingDir:   dir,
				ComposeFiles: []string{file},
				Environment:  types.Mapping{"TAG_VAR": "v1"},
			}

			reloaded, err := compose.ReloadWithNewName(
				context.Background(),
				project,
				"my-namespace",
			)

			Expect(err).ToNot(HaveOccurred())
			Expect(reloaded.Services["api"].Image).To(Equal("reg/api:v1"))
		})

		It("leaves the caller's environment untouched", func() {
			dir := GinkgoT().TempDir()
			file := filepath.Join(dir, "compose.yml")

			Expect(os.WriteFile(file, []byte(composeFile), 0o600)).To(Succeed())

			env := types.Mapping{"GREETING_VAR": "hello"}
			project := config.ComposeProject{
				Name:         "original",
				WorkingDir:   dir,
				ComposeFiles: []string{file},
				Environment:  env,
			}

			_, err := compose.ReloadWithNewName(
				context.Background(),
				project,
				"my-namespace",
			)

			Expect(err).ToNot(HaveOccurred())
			Expect(env).To(Equal(types.Mapping{"GREETING_VAR": "hello"}))
		})
	})

	Describe("RemoveServices", func() {
		It("removes the named services", func() {
			project := newProject()

			compose.RemoveServices(project, []string{"job"})

			Expect(project.Services).ToNot(HaveKey("job"))
			Expect(project.Services).To(HaveKey("api"))
			Expect(project.Services).To(HaveKey("db"))
		})

		It("prunes depends_on edges pointing at a removed service", func() {
			project := newProject()

			compose.RemoveServices(project, []string{"job"})

			Expect(project.Services["api"].DependsOn).ToNot(HaveKey("job"))
		})

		It("leaves depends_on edges to surviving services intact", func() {
			project := newProject()

			compose.RemoveServices(project, []string{"job"})

			Expect(project.Services["api"].DependsOn).To(HaveKey("db"))
			Expect(project.Services["api"].DependsOn["db"].Condition).
				To(Equal("service_healthy"))
		})

		It("removes several services at once", func() {
			project := newProject()

			compose.RemoveServices(project, []string{"job", "db"})

			Expect(project.Services).To(HaveLen(1))
			Expect(project.Services["api"].DependsOn).To(BeEmpty())
		})

		It("is a no-op for an empty name list", func() {
			project := newProject()

			compose.RemoveServices(project, []string{})

			Expect(project.Services).To(HaveLen(3))
			Expect(project.Services["api"].DependsOn).To(HaveLen(2))
		})

		It("is a no-op for a name that is not in the project", func() {
			project := newProject()

			compose.RemoveServices(project, []string{"nope"})

			Expect(project.Services).To(HaveLen(3))
			Expect(project.Services["api"].DependsOn).To(HaveLen(2))
		})
	})

	Describe("InjectNgrokService", func() {
		It("adds an ngrok service carrying the environment variable that pulls from host", func() {
			project := newProject()
			compose.InjectNgrokService(project, "/remote/ngrok.yml", "sum")
			ngrok, ngrokOk := project.Services["ngrok"]
			ngrokToken, ngrokTokenOk := ngrok.Environment["NGROK_AUTHTOKEN"]

			Expect(ngrokOk).To(BeTrue(), "no ngrok service was injected")
			Expect(ngrok.Image).To(
				Equal(config.NgrokImageRepo + ":" + config.NgrokImageTag))
			Expect(ngrokTokenOk).To(BeTrue())
			Expect(ngrokToken).To(BeNil())
		})

		It("mounts the remote config read-only at the agent's config path", func() {
			project := newProject()

			compose.InjectNgrokService(project, "/remote/ngrok.yml", "sum")

			vols := project.Services["ngrok"].Volumes
			Expect(vols).To(HaveLen(1))
			Expect(vols[0].Source).To(Equal("/remote/ngrok.yml"))
			Expect(vols[0].Target).To(Equal(
				config.NgrokConfigVolMountPath + "/" + config.NgrokConfigKey,
			))
			Expect(vols[0].ReadOnly).To(BeTrue())
		})

		It("depends on every service present at injection time", func() {
			project := newProject()

			compose.InjectNgrokService(project, "/remote/ngrok.yml", "sum")

			Expect(project.Services["ngrok"].DependsOn).
				To(HaveLen(3))
			Expect(project.Services["ngrok"].DependsOn).
				To(HaveKey("api"))
		})

		// Guards the ordering in Docker.modifyComposeContent: injecting before
		// removal would re-add an edge to a service that is not deployed.
		It("does not depend on a service removed beforehand", func() {
			project := newProject()

			compose.RemoveServices(project, []string{"job"})
			compose.InjectNgrokService(project, "/remote/ngrok.yml", "sum")

			Expect(project.Services["ngrok"].DependsOn).To(HaveLen(2))
			Expect(project.Services["ngrok"].DependsOn).ToNot(HaveKey("job"))
		})
	})

	Describe("SetImages", func() {
		It("leaves an existing image alone", func() {
			project := newProject()

			err := compose.SetImages(project, map[string]config.ServiceImage{
				"api": {Repository: "repo/api", Tag: "v1"},
				"job": {Repository: "repo/job", Tag: "v1"},
				"db":  {Repository: "repo/db", Tag: "v1"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(project.Services["job"].Image).To(Equal("alpine:3.21"))
		})

		It("fills in the image from the extension when the service has none", func() {
			project := newProject()

			err := compose.SetImages(project, map[string]config.ServiceImage{
				"api": {Repository: "repo/api", Tag: "v1"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(project.Services["api"].Image).To(Equal("repo/api:v1"))
		})

		It("errors when a service without an image is absent from the map", func() {
			project := newProject()

			err := compose.SetImages(project, map[string]config.ServiceImage{})

			Expect(err).To(MatchError(compose.ErrMissingService))
		})

		It("errors when the extension image is incomplete", func() {
			project := newProject()

			err := compose.SetImages(project, map[string]config.ServiceImage{
				"api": {Repository: "repo/api"},
			})

			Expect(err).To(MatchError(compose.ErrMissingImage))
		})

		// A skipped service is removed before SetImages runs, so its missing
		// image must not fail the deploy.
		It("ignores a service that was removed beforehand", func() {
			project := newProject()

			compose.RemoveServices(project, []string{"api"})

			err := compose.SetImages(project, map[string]config.ServiceImage{})

			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("the clearing helpers", func() {
		It("strips port publishing", func() {
			project := newProject()

			compose.ClearPortMappings(project)

			Expect(project.Services["api"].Ports).To(BeEmpty())
		})

		It("strips build settings", func() {
			project := newProject()

			compose.ClearBuildSettings(project)

			Expect(project.Services["api"].Build).To(BeNil())
		})

		It("drops bind volumes but keeps named ones", func() {
			project := newProject()

			compose.ClearServiceVolumes(project)

			vols := project.Services["api"].Volumes
			Expect(vols).To(HaveLen(1))
			Expect(vols[0].Type).To(Equal("volume"))
			Expect(vols[0].Source).To(Equal("data"))
		})

		// Clearing is only safe because the values survive the drop; a spec
		// that checked the nils alone would pass on a rewrite that took the
		// resolved variables with them.
		It("drops env and label files but keeps what they resolved to", func() {
			project := newProject()

			compose.ClearEnvAndLabelFiles(project)

			api := project.Services["api"]
			Expect(api.EnvFiles).To(BeEmpty())
			Expect(api.LabelFiles).To(BeEmpty())
			Expect(api.Environment).To(HaveKey("FROM_ENV_FILE"))
			Expect(api.Labels).To(HaveKeyWithValue("from.label.file", "yes"))
		})

		It("nils out empty commands and entrypoints", func() {
			project := newProject()

			compose.ClearEmptyCommandsAndEntryPoints(project)

			Expect(project.Services["api"].Command).To(BeNil())
			Expect(project.Services["api"].Entrypoint).To(BeNil())
		})

		It("clears an empty command next to a populated entrypoint", func() {
			project := newProject()
			svc := project.Services["api"]
			svc.Entrypoint = types.ShellCommand{"/bin/app"}
			project.Services["api"] = svc

			compose.ClearEmptyCommandsAndEntryPoints(project)

			Expect(project.Services["api"].Command).To(BeNil())
			Expect(project.Services["api"].Entrypoint).
				To(Equal(types.ShellCommand{"/bin/app"}))
		})

		It("clears an empty entrypoint next to a populated command", func() {
			project := newProject()
			svc := project.Services["api"]
			svc.Command = types.ShellCommand{"serve"}
			project.Services["api"] = svc

			compose.ClearEmptyCommandsAndEntryPoints(project)

			Expect(project.Services["api"].Entrypoint).To(BeNil())
			Expect(project.Services["api"].Command).
				To(Equal(types.ShellCommand{"serve"}))
		})
	})

	Describe("Marshal", func() {
		It("renders the fully rewritten project as valid yaml", func() {
			project := newProject()

			compose.RemoveServices(project, []string{"job"})
			compose.ClearBuildSettings(project)
			compose.ClearPortMappings(project)
			compose.ClearServiceVolumes(project)
			compose.ClearEnvAndLabelFiles(project)
			compose.ClearEmptyCommandsAndEntryPoints(project)
			compose.InjectNgrokService(project, "/remote/ngrok.yml", "sum")

			Expect(compose.SetImages(project, map[string]config.ServiceImage{
				"api": {Repository: "repo/api", Tag: "v1"},
			})).To(Succeed())

			out, err := compose.Marshal(project)
			Expect(err).ToNot(HaveOccurred())

			var parsed map[string]any
			Expect(yaml.Unmarshal(out, &parsed)).
				To(Succeed(), "marshalled project is not valid yaml: %s", out)

			services, ok := parsed["services"].(map[string]any)
			Expect(ok).To(BeTrue(), "no services block in %s", out)
			Expect(services).To(HaveKey("ngrok"))
			Expect(services).ToNot(HaveKey("job"))

			// These name paths on the developer's machine, and the remote
			// fails the whole project rather than skipping one it cannot find.
			api, ok := services["api"].(map[string]any)
			Expect(ok).To(BeTrue(), "no api service in %s", out)
			Expect(api).ToNot(HaveKey("env_file"))
			Expect(api).ToNot(HaveKey("label_file"))
			Expect(api).To(HaveKey("environment"))
		})

		It("escapes a literal dollar so the remote cannot re-interpolate it", func() {
			project := newProject()
			svc := project.Services["db"]
			svc.Environment = types.MappingWithEquals{
				"POSTGRES_PASSWORD": new("p$ssw0rd"),
			}
			project.Services["db"] = svc

			out, err := compose.Marshal(project)

			Expect(err).ToNot(HaveOccurred())
			Expect(string(out)).To(ContainSubstring("p$$ssw0rd"))
			Expect(string(out)).ToNot(ContainSubstring("p${"))
		})
	})
})
