package compose_test

import (
	"context"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/compose"
	"github.com/robgonnella/minienv/internal/config"
)

const testdataDir = "testdata"

func fixture(parts ...string) string {
	return filepath.Join(append([]string{testdataDir}, parts...)...)
}

func absFixture(parts ...string) string {
	GinkgoHelper()

	abs, err := filepath.Abs(fixture(parts...))
	Expect(err).ShouldNot(HaveOccurred())

	return abs
}

// ProjectDirectory is pinned to the fixture's directory so the repo's own .env
// cannot leak into these specs.
func loadIn(dir string, files ...string) *compose.Project {
	GinkgoHelper()

	project, err := compose.Load(context.Background(), compose.Source{
		Files:            files,
		ProjectDirectory: dir,
		ProjectName:      "test-project",
	})
	Expect(err).ShouldNot(HaveOccurred())

	return project
}

var _ = Describe("Load", func() {
	It("errors when the compose file does not exist", func() {
		_, err := compose.Load(context.Background(), compose.Source{
			Files:            []string{fixture("nope.compose.yml")},
			ProjectDirectory: testdataDir,
		})

		Expect(err).To(MatchError(compose.ErrProjectLoad))
	})

	// A reserved ngrok URL is committed to compose.yml, so it can only differ
	// per developer if compose interpolates it.
	Describe("interpolation", func() {
		BeforeEach(func() {
			GinkgoT().Setenv("MINIENV_NAMESPACE", "alice")
		})

		It("resolves env vars inside a service extension", func() {
			project := loadIn(testdataDir, fixture("interpolated.compose.yml"))

			ext := project.Services["api"].
				Extensions[config.K8sServiceExtension].(map[string]any)
			ngrok := ext["ngrok"].(map[string]any)

			Expect(ngrok["url"]).
				To(Equal("https://alice-api.example.ngrok.app"))
		})

		It("resolves env vars inside the top level extension", func() {
			project := loadIn(testdataDir, fixture("interpolated.compose.yml"))

			ext := project.
				Extensions[config.TopLevelExtension].(map[string]any)
			k8s := ext["k8s"].(map[string]any)

			Expect(k8s["namespace"]).To(Equal("alice"))
		})
	})
})

var _ = Describe("LoadServiceDirs", func() {
	includeDir := fixture("include")

	serviceDirs := func(dir, file string) compose.ServiceDirs {
		GinkgoHelper()

		dirs, err := compose.LoadServiceDirs(
			context.Background(),
			loadIn(dir, file),
		)
		Expect(err).ShouldNot(HaveOccurred())

		return dirs
	}

	It("maps every service to the project directory without include", func() {
		dirs := serviceDirs(testdataDir, fixture("interpolated.compose.yml"))

		Expect(dirs).To(Equal(compose.ServiceDirs{
			"api": absFixture(),
		}))
	})

	Context("with included files", func() {
		var dirs compose.ServiceDirs

		BeforeEach(func() {
			dirs = serviceDirs(
				filepath.Join(includeDir, "main"),
				filepath.Join(includeDir, "main", "compose.yml"),
			)
		})

		It("maps a top-level service to the project directory", func() {
			Expect(dirs.Dir("web")).To(Equal(absFixture("include", "main")))
		})

		It("maps an included service to the included file's directory", func() {
			Expect(dirs.Dir("api")).To(Equal(absFixture("include", "other")))
		})

		It("follows an include nested inside an included file", func() {
			Expect(dirs.Dir("worker")).
				To(Equal(absFixture("include", "other", "deeper")))
		})

		It("honours an include's project_directory", func() {
			Expect(dirs.Dir("job")).
				To(Equal(absFixture("include", "nested", "base")))
		})

		It("covers every service compose loaded", func() {
			Expect(dirs).To(HaveLen(4))
		})
	})

	It("attributes a service declared in both files to the includer", func() {
		dirs := serviceDirs(
			filepath.Join(includeDir, "override"),
			filepath.Join(includeDir, "override", "compose.yml"),
		)

		Expect(dirs.Dir("api")).To(Equal(absFixture("include", "override")))
		Expect(dirs.Dir("worker")).
			To(Equal(absFixture("include", "other", "deeper")))
	})

	It("interpolates an included file with the include's env_file", func() {
		dirs := serviceDirs(
			filepath.Join(includeDir, "envfile"),
			filepath.Join(includeDir, "envfile", "compose.yml"),
		)

		Expect(dirs).To(Equal(compose.ServiceDirs{
			"web":  absFixture("include", "envfile"),
			"sub":  absFixture("include", "envfile", "sub"),
			"leaf": absFixture("include", "envfile", "sub", "deeper"),
		}))
	})

	It("errors for a service the walk never saw", func() {
		project := loadIn(testdataDir, fixture("interpolated.compose.yml"))
		project.Services["ghost"] = compose.Service{Name: "ghost"}

		_, err := compose.LoadServiceDirs(context.Background(), project)

		Expect(err).To(MatchError(compose.ErrServiceDirMissing))
	})
})

var _ = Describe("LoadRawServices", func() {
	hostenvDir := fixture("hostenv")

	var raw compose.RawServices

	BeforeEach(func() {
		GinkgoT().Setenv("HOST_VALUE", "from-host")
		GinkgoT().Setenv("PASSTHROUGH", "passed")

		services, err := compose.LoadRawServices(
			context.Background(),
			loadIn(hostenvDir, filepath.Join(hostenvDir, "compose.yml")),
		)
		Expect(err).ShouldNot(HaveOccurred())

		raw = services
	})

	It("keeps a list-form environment uninterpolated", func() {
		Expect(raw.Get("api").Environment).To(Equal([]any{
			"LITERAL=plain",
			"ESCAPED=$$not-a-variable",
			"FROM_HOST=${HOST_VALUE}",
			"WITH_DEFAULT=${HOSTENV_UNSET_VALUE:-fallback}",
			"PASSTHROUGH",
			"HOSTENV_UNSET_PASSTHROUGH",
		}))
	})

	It("keeps a map-form environment uninterpolated", func() {
		Expect(raw.Get("worker").Environment).To(Equal(map[string]any{
			"PORT":      8080,
			"DEBUG":     "true",
			"TOKEN":     "${HOST_VALUE}",
			"INHERITED": nil,
		}))
	})

	It("carries every extension block under the service intact", func() {
		Expect(raw.Get("api").Extensions).To(Equal(map[string]any{
			config.K8sServiceExtension: map[string]any{
				"env": []any{
					map[string]any{"name": "API_KEY", "value": "${HOST_VALUE}"},
					map[string]any{"name": "LOG_LEVEL", "value": "debug"},
				},
			},
		}))
	})

	It("carries no extensions for a service declaring none", func() {
		Expect(raw.Get("worker").Extensions).To(BeEmpty())
	})

	It("covers a service declared in an included file", func() {
		Expect(raw.Get("sub").Environment).To(Equal([]any{
			"SUB_SECRET=${HOST_VALUE}",
			"SUB_LITERAL=plain",
		}))
	})

	It("still has an entry for a service whose values are all literal", func() {
		Expect(raw).To(HaveKey("plain"))
	})

	It("has no entry for a service that was never declared", func() {
		Expect(raw.Get("ghost")).To(Equal(compose.RawService{}))
	})
})

var _ = Describe("Templated", func() {
	DescribeTable(
		"reports whether compose would interpolate the value",
		func(value string, want bool) {
			Expect(compose.Templated(value)).To(Equal(want))
		},
		Entry("a braced variable", "${VAR}", true),
		Entry("a bare variable", "$VAR", true),
		Entry("a variable with a default", "${VAR:-fallback}", true),
		Entry("a variable inside text", "prefix-${VAR}-suffix", true),
		Entry("a plain value", "plain", false),
		Entry("an escaped dollar", "$$not-a-variable", false),
		Entry("an escaped dollar next to a real one", "$$literal-${VAR}", true),
		Entry("an empty value", "", false),
	)
})

var _ = Describe("LiteralEnvKeys", func() {
	DescribeTable(
		"names the keys whose values are written literally",
		func(raw any, want map[string]bool) {
			Expect(compose.LiteralEnvKeys(raw)).To(Equal(want))
		},
		Entry(
			"a list form",
			[]any{
				"LITERAL=plain",
				"ESCAPED=$$not-a-variable",
				"FROM_HOST=${HOST_VALUE}",
				"PASSTHROUGH",
				42,
			},
			map[string]bool{"LITERAL": true, "ESCAPED": true},
		),
		Entry(
			"a map form",
			map[string]any{
				"PORT":      8080,
				"DEBUG":     "true",
				"TOKEN":     "${HOST_VALUE}",
				"INHERITED": nil,
			},
			map[string]bool{"PORT": true, "DEBUG": true},
		),
		Entry("an absent environment", nil, map[string]bool{}),
		Entry("an unexpected shape", "FOO=bar", map[string]bool{}),
	)
})
