package image_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/image"
)

func helloService() image.ServiceProperties {
	return image.ServiceProperties{
		Name:       "hello",
		Registry:   "reg/hello",
		Tag:        "v1",
		Context:    ".",
		Dockerfile: "Dockerfile",
		Platforms:  []string{"linux/amd64"},
	}
}

var _ = Describe("HclBuilder", func() {
	var (
		subject  *image.HclBuilder
		services []image.ServiceProperties
	)

	BeforeEach(func() {
		subject = image.NewHclBuilder()
		services = []image.ServiceProperties{helloService()}
	})

	Describe("Build", func() {
		// Asserted whole rather than by substring: buildx parses this as hcl,
		// so stray escaping or indentation anywhere in the document breaks the
		// build even when every individual field looks right.
		It("renders a complete bake file for a single service", func() {
			hcl, err := subject.Build(services)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(hcl).To(Equal(`
group "default" {
  targets = ["hello"]
}

target "hello" {
  context = "."
  dockerfile = "Dockerfile"
  tags = ["reg/hello:v1"]
  platforms = ["linux/amd64"]
}
`))
		})

		It("lists every service as a target", func() {
			services = append(services, image.ServiceProperties{
				Name:       "world",
				Registry:   "reg/world",
				Tag:        "v2",
				Context:    "./world",
				Dockerfile: "world/Dockerfile",
				Platforms:  []string{"linux/amd64", "linux/arm64"},
			})

			hcl, err := subject.Build(services)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(hcl).To(ContainSubstring(`targets = ["hello", "world"]`))
			Expect(hcl).To(ContainSubstring(`target "world" {`))
			Expect(hcl).To(ContainSubstring(`context = "./world"`))
			Expect(hcl).To(ContainSubstring(`dockerfile = "world/Dockerfile"`))
			Expect(hcl).To(ContainSubstring(`tags = ["reg/world:v2"]`))
		})

		It("quotes every entry of a multi-platform list", func() {
			services[0].Platforms = []string{"linux/amd64", "linux/arm64"}

			hcl, err := subject.Build(services)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(hcl).To(
				ContainSubstring(`platforms = ["linux/amd64", "linux/arm64"]`),
			)
		})

		It("renders build args sorted by key", func() {
			services[0].Args = map[string]string{
				"VERSION": "1.2.3",
				"COMMIT":  "abc1234",
			}

			hcl, err := subject.Build(services)

			// Keys are emitted in sorted order so the hcl is byte-stable
			// across runs despite randomized go map iteration.
			Expect(err).ShouldNot(HaveOccurred())
			Expect(hcl).To(ContainSubstring(
				"  args = {\n" +
					"    \"COMMIT\" = \"abc1234\"\n" +
					"    \"VERSION\" = \"1.2.3\"\n" +
					"  }\n",
			))
		})

		It("omits the args block entirely when a service has none", func() {
			hcl, err := subject.Build(services)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(hcl).ToNot(ContainSubstring("args = {"))
		})

		It("renders an empty target list for no services", func() {
			hcl, err := subject.Build(nil)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(hcl).To(ContainSubstring("targets = []"))
		})

		It("does not mutate the services it was given", func() {
			first, err := subject.Build(services)
			Expect(err).ShouldNot(HaveOccurred())

			firstServiceBefore := services[0]

			second, err := subject.Build(services)
			Expect(err).ShouldNot(HaveOccurred())

			firstServiceAfter := services[0]

			Expect(second).To(Equal(first))
			Expect(firstServiceBefore).To(Equal(firstServiceAfter))
		})
	})
})
