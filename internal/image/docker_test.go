package image_test

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/image"
)

var _ = Describe("ServiceProperties", func() {
	It("exposes its build fields for structured logging", func() {
		svc := helloService()

		Expect(svc.LogFields()).To(Equal(map[string]any{
			"name":       "hello",
			"registry":   "reg/hello",
			"tag":        "v1",
			"context":    ".",
			"dockerfile": "Dockerfile",
			"platforms":  []string{"linux/amd64"},
		}))
	})
})

var _ = Describe("Docker", func() {
	Describe("BakeArgs", func() {
		It("pushes when not a dry run", func() {
			Expect(image.NewDocker(false).BakeArgs()).
				To(Equal([]string{"buildx", "bake", "--push", "-f", "-"}))
		})

		It("omits --push on a dry run", func() {
			Expect(image.NewDocker(true).BakeArgs()).
				To(Equal([]string{"buildx", "bake", "-f", "-"}))
		})
	})

	Describe("GetFilteredList", func() {
		It("keeps services with every required build field", func() {
			services := []image.ServiceProperties{helloService()}

			Expect(image.NewDocker(true).GetFilteredList(services)).
				To(Equal(services))
		})

		DescribeTable("drops a service missing a required build field",
			func(mutate func(*image.ServiceProperties)) {
				svc := helloService()
				mutate(&svc)

				Expect(image.NewDocker(true).
					GetFilteredList([]image.ServiceProperties{svc})).
					To(BeEmpty())
			},
			Entry("no registry", func(s *image.ServiceProperties) {
				s.Registry = ""
			}),
			Entry("no tag", func(s *image.ServiceProperties) { s.Tag = "" }),
			Entry("no context", func(s *image.ServiceProperties) {
				s.Context = ""
			}),
			Entry("no dockerfile", func(s *image.ServiceProperties) {
				s.Dockerfile = ""
			}),
		)

		It("preserves the order of the services it keeps", func() {
			second := helloService()
			second.Name = "world"

			services := []image.ServiceProperties{
				helloService(),
				{Name: "incomplete"},
				second,
			}

			filtered := image.NewDocker(true).GetFilteredList(services)

			Expect(filtered).To(HaveLen(2))
			Expect(filtered[0].Name).To(Equal("hello"))
			Expect(filtered[1].Name).To(Equal("world"))
		})
	})

	Describe("BuildAndPush", func() {
		// These return before exec.Command, so they run without docker. Any
		// service reaching the shell-out would fail the suite on a machine
		// with no docker installed.
		It("does nothing when given no services", func() {
			Expect(image.NewDocker(true).BuildAndPush(context.Background(), nil)).To(Succeed())
		})

		It("does nothing when every service is filtered out", func() {
			Expect(image.NewDocker(true).BuildAndPush(
				context.Background(),
				[]image.ServiceProperties{{Name: "incomplete"}},
			)).To(Succeed())
		})
	})
})
