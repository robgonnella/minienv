package transport_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/transport"
)

// Any non-zero status; the value itself carries no meaning to the code.
const remoteFailureExitCode = 3

var _ = Describe("SSHTransport", func() {
	Describe("validating the config", func() {
		It("rejects a missing host", func() {
			_, err := transport.NewSSHTransport(transport.SSHTransportOptions{
				Config: config.XMiniEnvSSH{Identity: "/key"},
			})

			Expect(err).To(MatchError(transport.ErrInvalidExtension))
		})

		It("rejects a missing identity", func() {
			_, err := transport.NewSSHTransport(transport.SSHTransportOptions{
				Config: config.XMiniEnvSSH{Host: "example.com"},
			})

			Expect(err).To(MatchError(transport.ErrInvalidExtension))
		})

		It("accepts a host and an identity", func() {
			client, err := transport.NewSSHTransport(
				transport.SSHTransportOptions{
					Config: config.XMiniEnvSSH{
						Host:     "example.com",
						Identity: "/key",
					},
				})

			Expect(err).ToNot(HaveOccurred())
			Expect(client.String()).To(Equal("SSH"))
		})
	})

	It("closes cleanly when nothing was ever dialed", func() {
		client, err := transport.NewSSHTransport(transport.SSHTransportOptions{
			Config: config.XMiniEnvSSH{Host: "example.com", Identity: "/key"},
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(client.Close()).To(Succeed())
	})

	Describe("against a live server", func() {
		var (
			srv     *testServer
			subject *transport.SSHTransport
		)

		BeforeEach(func() {
			srv = newTestServer()
			trustHost(srv)

			var err error

			subject, err = transport.NewSSHTransport(
				transport.SSHTransportOptions{
					Config: config.XMiniEnvSSH{
						Host:     "127.0.0.1",
						Port:     srv.Port(),
						User:     "tester",
						Identity: writeIdentity(GinkgoT().TempDir()),
					},
				})
			Expect(err).ToNot(HaveOccurred())

			DeferCleanup(func() { _ = subject.Close() })
		})

		Describe("connecting", func() {
			It("dials with the configured identity", func() {
				Expect(subject.RunCommand("true")).To(Succeed())
				Expect(srv.Commands()).To(ConsistOf("true"))
			})

			It("reuses one connection across calls", func() {
				Expect(subject.RunCommand("one")).To(Succeed())
				Expect(subject.RunCommand("two")).To(Succeed())

				Expect(srv.Commands()).To(Equal([]string{"one", "two"}))
			})

			It("closes after a real dial", func() {
				Expect(subject.RunCommand("true")).To(Succeed())
				Expect(subject.Close()).To(Succeed())
			})

			It("stays closeable twice", func() {
				Expect(subject.RunCommand("true")).To(Succeed())
				Expect(subject.Close()).To(Succeed())
				Expect(subject.Close()).To(Succeed())
			})

			It("reports a refused session", func() {
				srv.RefuseSessions()

				Expect(subject.RunCommand("true")).
					To(MatchError(transport.ErrSSHSessionCreate))
			})
		})

		Describe("RunCommand", func() {
			It("returns nil on a zero exit", func() {
				Expect(subject.RunCommand("true")).To(Succeed())
			})

			It("reports a non-zero exit", func() {
				srv.FailWith(remoteFailureExitCode)

				Expect(subject.RunCommand("false")).
					To(MatchError(transport.ErrSSHCommandRun))
			})

			It("reports a plain command in full", func() {
				srv.FailWith(remoteFailureExitCode)

				err := subject.RunCommand("docker compose up -d")

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("docker compose up -d"))
			})
		})

		// ErrSSHCommandRun is logged in full further up, so anything a command
		// body carries reaches the log with it.
		Describe("what a failure reports", func() {
			const secret = "2abcTESTTOKENvalue_do_not_log"

			BeforeEach(func() {
				srv.FailWith(remoteFailureExitCode)
			})

			It("redacts a heredoc body", func() {
				err := subject.CreateFile("/remote/.env", []byte(
					"NGROK_AUTHTOKEN="+secret,
				))

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).ToNot(ContainSubstring(secret))
				Expect(err.Error()).To(ContainSubstring("<REDACTED>"))
			})

			// Redacting the whole command would leave no way to tell which
			// write failed.
			It("keeps the path of the file it was writing", func() {
				err := subject.CreateFile("/remote/.env", []byte(secret))

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("/remote/.env"))
			})

			It("redacts a body spanning several lines", func() {
				err := subject.CreateFile("/remote/compose.yml", []byte(
					"services:\n  api:\n    environment:\n      TOKEN: "+
						secret+"\n",
				))

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).ToNot(ContainSubstring(secret))
			})
		})

		Describe("CreateFile", func() {
			It("creates the parent directory before writing", func() {
				Expect(subject.CreateFile("/remote/dir/compose.yml", []byte("a"))).
					To(Succeed())

				Expect(srv.Commands()).To(HaveLen(1))
				Expect(srv.Commands()[0]).
					To(HavePrefix("mkdir -p /remote/dir && cat << "))
			})

			It("redirects the heredoc at the requested path", func() {
				Expect(subject.CreateFile("/remote/compose.yml", []byte("a"))).
					To(Succeed())

				Expect(srv.Commands()[0]).
					To(ContainSubstring("> /remote/compose.yml"))
			})

			It("writes the body between the delimiters", func() {
				body := "services:\n  api:\n    image: repo/api:v1\n"

				Expect(subject.CreateFile("/remote/compose.yml", []byte(body))).
					To(Succeed())

				Expect(heredocBody(srv.Commands()[0])).To(Equal(body))
			})

			It("quotes the delimiter so the remote shell cannot expand the body", func() {
				body := "GREETING=$HOME\nSUB=`id`\nOTHER=$(whoami)\n"

				Expect(subject.CreateFile("/remote/.env", []byte(body))).
					To(Succeed())

				cmd := srv.Commands()[0]

				Expect(cmd).To(MatchRegexp(`cat << 'MINIENV_[0-9a-f]+' >`))
				Expect(heredocBody(cmd)).To(Equal(body))
			})

			// A fixed "EOF" would truncate any file containing a bare EOF line.
			It("survives a body containing a bare EOF line", func() {
				body := "before\nEOF\nafter\n"

				Expect(subject.CreateFile("/remote/compose.yml", []byte(body))).
					To(Succeed())

				Expect(heredocBody(srv.Commands()[0])).To(Equal(body))
			})

			It("uses a fresh delimiter on every call", func() {
				Expect(subject.CreateFile("/remote/a", []byte("a"))).To(Succeed())
				Expect(subject.CreateFile("/remote/b", []byte("b"))).To(Succeed())

				first := heredocDelimiterOf(srv.Commands()[0])
				second := heredocDelimiterOf(srv.Commands()[1])

				Expect(first).To(HavePrefix("MINIENV_"))
				Expect(second).To(HavePrefix("MINIENV_"))
				Expect(first).ToNot(Equal(second))
			})

			It("reports a non-zero exit from the remote write", func() {
				srv.FailWith(remoteFailureExitCode)

				Expect(subject.CreateFile("/remote/compose.yml", []byte("a"))).
					To(MatchError(transport.ErrSSHCommandRun))
			})
		})
	})

	Describe("resolving the identity", func() {
		var (
			srv  *testServer
			home string
		)

		BeforeEach(func() {
			srv = newTestServer()
			home = trustHost(srv)
		})

		connectWith := func(identity string) error {
			client, err := transport.NewSSHTransport(
				transport.SSHTransportOptions{
					Config: config.XMiniEnvSSH{
						Host:     "127.0.0.1",
						Port:     srv.Port(),
						User:     "tester",
						Identity: identity,
					},
				})
			Expect(err).ToNot(HaveOccurred())

			DeferCleanup(func() { _ = client.Close() })

			return client.RunCommand("true")
		}

		It("reads a key given by a relative path", func() {
			dir := GinkgoT().TempDir()
			identity := writeIdentity(dir)

			rel, err := filepath.Rel(mustGetwd(), identity)
			Expect(err).ToNot(HaveOccurred())

			Expect(connectWith(rel)).To(Succeed())
		})

		// Every documented example writes the identity this way.
		It("expands a leading ~ against the home directory", func() {
			writeIdentity(filepath.Join(home, ".ssh"))

			Expect(connectWith("~/.ssh/id_ed25519")).To(Succeed())
		})

		It("reports a missing identity file", func() {
			missing := filepath.Join(GinkgoT().TempDir(), "nope")

			Expect(connectWith(missing)).
				To(MatchError(transport.ErrSSHIdentityRead))
		})

		It("reports an identity that is not a key", func() {
			path := filepath.Join(GinkgoT().TempDir(), "garbage")
			Expect(os.WriteFile(path, []byte("not a key"), 0o600)).To(Succeed())

			Expect(connectWith(path)).
				To(MatchError(transport.ErrSSHSignerCreate))
		})
	})

	Describe("verifying the host key", func() {
		It("refuses a host that known_hosts does not list", func() {
			srv := newTestServer()

			// A syntactically valid entry for a host that is not this one.
			writeKnownHosts(
				"[127.0.0.1]:1 ssh-ed25519 " +
					"AAAAC3NzaC1lZDI1NTE5AAAAIGkQz6UqJ3vXrH5YHCvJ2bm0nM8V5hV/" +
					"cKq0xJZ8mVxL",
			)

			client, err := transport.NewSSHTransport(
				transport.SSHTransportOptions{
					Config: config.XMiniEnvSSH{
						Host:     "127.0.0.1",
						Port:     srv.Port(),
						User:     "tester",
						Identity: writeIdentity(GinkgoT().TempDir()),
					},
				})
			Expect(err).ToNot(HaveOccurred())

			DeferCleanup(func() { _ = client.Close() })

			Expect(client.RunCommand("true")).
				To(MatchError(transport.ErrSSHClientCreate))
		})

		It("reports an unreadable known_hosts", func() {
			srv := newTestServer()

			// HOME with no .ssh/known_hosts at all.
			GinkgoT().Setenv("HOME", GinkgoT().TempDir())

			client, err := transport.NewSSHTransport(
				transport.SSHTransportOptions{
					Config: config.XMiniEnvSSH{
						Host:     "127.0.0.1",
						Port:     srv.Port(),
						User:     "tester",
						Identity: writeIdentity(GinkgoT().TempDir()),
					},
				})
			Expect(err).ToNot(HaveOccurred())

			DeferCleanup(func() { _ = client.Close() })

			Expect(client.RunCommand("true")).
				To(MatchError(transport.ErrSSHHostKeyCallback))
		})

		It("reports a home directory it cannot resolve", func() {
			srv := newTestServer()

			// os.UserHomeDir treats an empty HOME as undefined.
			GinkgoT().Setenv("HOME", "")

			client, err := transport.NewSSHTransport(
				transport.SSHTransportOptions{
					Config: config.XMiniEnvSSH{
						Host:     "127.0.0.1",
						Port:     srv.Port(),
						User:     "tester",
						Identity: writeIdentity(GinkgoT().TempDir()),
					},
				})
			Expect(err).ToNot(HaveOccurred())

			DeferCleanup(func() { _ = client.Close() })

			Expect(client.RunCommand("true")).
				To(MatchError(transport.ErrUserHomeDir))
		})
	})
})

func heredocDelimiterOf(cmd string) string {
	GinkgoHelper()

	_, rest, found := strings.Cut(cmd, "<< '")
	Expect(found).To(BeTrue(), "no heredoc in %q", cmd)

	delimiter, _, found := strings.Cut(rest, "'")
	Expect(found).To(BeTrue(), "unterminated delimiter in %q", cmd)

	return delimiter
}

// Asserts on what the remote would write, not on the command's shape.
func heredocBody(cmd string) string {
	GinkgoHelper()

	delimiter := heredocDelimiterOf(cmd)

	_, body, found := strings.Cut(cmd, "\n")
	Expect(found).To(BeTrue(), "no heredoc body in %q", cmd)

	body, found = strings.CutSuffix(body, "\n"+delimiter)
	Expect(found).To(BeTrue(), "body not terminated by %q", delimiter)

	return body
}

func mustGetwd() string {
	GinkgoHelper()

	wd, err := os.Getwd()
	Expect(err).ToNot(HaveOccurred())

	return wd
}
