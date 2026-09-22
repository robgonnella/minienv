package transport_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/transport"
)

// Any non-zero status; the value itself carries no meaning to the code.
const remoteFailureExitCode = 3

// Long enough for the call to be blocked on the server when it fires.
const cancelAfter = 200 * time.Millisecond

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
				Expect(subject.RunCommand(context.Background(), "true")).To(Succeed())
				Expect(srv.Commands()).To(ConsistOf("true"))
			})

			It("reuses one connection across calls", func() {
				Expect(subject.RunCommand(context.Background(), "one")).To(Succeed())
				Expect(subject.RunCommand(context.Background(), "two")).To(Succeed())

				Expect(srv.Commands()).To(Equal([]string{"one", "two"}))
			})

			It("closes after a real dial", func() {
				Expect(subject.RunCommand(context.Background(), "true")).To(Succeed())
				Expect(subject.Close()).To(Succeed())
			})

			It("stays closeable twice", func() {
				Expect(subject.RunCommand(context.Background(), "true")).To(Succeed())
				Expect(subject.Close()).To(Succeed())
				Expect(subject.Close()).To(Succeed())
			})

			It("reports a refused session", func() {
				srv.RefuseSessions()

				Expect(subject.RunCommand(context.Background(), "true")).
					To(MatchError(transport.ErrSSHSessionCreate))
			})
		})

		Describe("RunCommand", func() {
			It("returns nil on a zero exit", func() {
				Expect(subject.RunCommand(context.Background(), "true")).To(Succeed())
			})

			It("reports a non-zero exit", func() {
				srv.FailWith(remoteFailureExitCode)

				Expect(subject.RunCommand(context.Background(), "false")).
					To(MatchError(transport.ErrSSHCommandRun))
			})

			It("reports a plain command in full", func() {
				srv.FailWith(remoteFailureExitCode)

				err := subject.RunCommand(context.Background(), "docker compose up -d")

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("docker compose up -d"))
			})
		})

		Describe("cancellation", func() {
			It("refuses an already-cancelled context without dialing", func() {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()

				err := subject.RunCommand(ctx, "true")

				Expect(err).To(MatchError(transport.ErrCancelled))
				Expect(err).To(MatchError(context.Canceled))
				Expect(srv.Commands()).To(BeEmpty())
			})

			It("returns once the context expires under a hanging command", func() {
				srv.Hang()

				ctx, cancel := context.WithTimeout(context.Background(), cancelAfter)
				defer cancel()

				err := subject.RunCommand(ctx, "sleep")

				Expect(err).To(MatchError(transport.ErrCancelled))
				Expect(err).To(MatchError(context.DeadlineExceeded))
				Expect(srv.Commands()).To(ConsistOf("sleep"))
			})

			It("redials after a cancelled call", func() {
				srv.Hang()

				ctx, cancel := context.WithTimeout(context.Background(), cancelAfter)
				defer cancel()

				Expect(subject.RunCommand(ctx, "one")).
					To(MatchError(transport.ErrCancelled))

				srv.Release()

				Expect(subject.RunCommand(context.Background(), "two")).To(Succeed())
				Expect(srv.Commands()).To(Equal([]string{"one", "two"}))
			})

			It("returns once the context expires under a hanging write", func() {
				Expect(subject.CreateFile(context.Background(), "~/a", []byte("a"))).
					To(Succeed())

				ctx, cancel := context.WithCancel(context.Background())
				cancel()

				Expect(subject.CreateFile(ctx, "~/b", []byte("b"))).
					To(MatchError(transport.ErrCancelled))
				Expect(filepath.Join(srv.SFTPHome(), "b")).ToNot(BeAnExistingFile())
			})
		})

		// ErrSSHCommandRun is logged in full further up, so anything a command
		// body carries reaches the log with it.
		Describe("what a failure reports", func() {
			BeforeEach(func() {
				srv.FailWith(remoteFailureExitCode)
			})

			It("masks a command that could be carrying a credential", func() {
				err := subject.RunCommand(context.Background(), "cat /etc/shadow")

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("cat <REDACTED>"))
				Expect(err.Error()).ToNot(ContainSubstring("/etc/shadow"))
			})
		})

		Describe("what a failed write reports", func() {
			const secret = "2abcTESTTOKENvalue_do_not_log"

			var blocked string

			BeforeEach(func() {
				blocked = filepath.Join(srv.SFTPHome(), "blocked")

				Expect(os.MkdirAll(blocked, 0o500)).To(Succeed())
			})

			It("says nothing about the content it was writing", func() {
				err := subject.CreateFile(
					context.Background(),
					blocked+"/.env",
					[]byte("NGROK_AUTHTOKEN="+secret),
				)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).ToNot(ContainSubstring(secret))
			})

			It("keeps the path of the file it was writing", func() {
				err := subject.CreateFile(context.Background(), blocked+"/.env", []byte(secret))

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(".env"))
			})
		})

		Describe("CreateFile", func() {
			landed := func(name string) string {
				return filepath.Join(srv.SFTPHome(), name)
			}

			It("creates the parent directory before writing", func() {
				Expect(subject.CreateFile(context.Background(), "~/dir/compose.yml", []byte("a"))).
					To(Succeed())

				Expect(landed("dir/compose.yml")).To(BeAnExistingFile())
			})

			It("writes the body exactly as given", func() {
				body := "services:\n  api:\n    image: repo/api:v1\n"

				Expect(subject.CreateFile(context.Background(), "~/compose.yml", []byte(body))).
					To(Succeed())

				Expect(os.ReadFile(landed("compose.yml"))).
					To(Equal([]byte(body)))
			})

			It("writes shell syntax as the literal text it is", func() {
				body := "GREETING=$HOME\nSUB=`id`\nOTHER=$(whoami)\n"

				Expect(subject.CreateFile(context.Background(), "~/.env", []byte(body))).To(Succeed())

				Expect(os.ReadFile(landed(".env"))).To(Equal([]byte(body)))
			})

			It("adds nothing to a body that ends without a newline", func() {
				Expect(subject.CreateFile(context.Background(), "~/compose.yml", []byte("a"))).
					To(Succeed())

				Expect(os.ReadFile(landed("compose.yml"))).To(Equal([]byte("a")))
			})

			It("replaces a longer body rather than writing over part of it", func() {
				Expect(subject.CreateFile(context.Background(), "~/compose.yml", []byte("a long body"))).
					To(Succeed())
				Expect(subject.CreateFile(context.Background(), "~/compose.yml", []byte("short"))).
					To(Succeed())

				Expect(os.ReadFile(landed("compose.yml"))).
					To(Equal([]byte("short")))
			})

			It("reports a path it cannot write", func() {
				blocked := landed("blocked")

				Expect(os.MkdirAll(blocked, 0o500)).To(Succeed())

				Expect(subject.CreateFile(context.Background(), blocked+"/compose.yml", []byte("a"))).
					To(MatchError(transport.ErrSFTPRemoteWrite))
			})
		})

		Describe("CopyPath", func() {
			var local string

			BeforeEach(func() {
				local = GinkgoT().TempDir()
			})

			writeLocal := func(name, body string, mode os.FileMode) string {
				GinkgoHelper()

				full := filepath.Join(local, name)

				Expect(os.MkdirAll(filepath.Dir(full), 0o700)).To(Succeed())
				Expect(os.WriteFile(full, []byte(body), mode)).To(Succeed())

				return full
			}

			landed := func(name string) string {
				return filepath.Join(srv.SFTPHome(), name)
			}

			It("reproduces a file byte for byte", func() {
				body := "declared: yes"
				file := writeLocal("app.yml", body, 0o600)

				Expect(subject.CopyPath(context.Background(), file, "~/copies/app.yml")).To(Succeed())

				Expect(os.ReadFile(landed("copies/app.yml"))).
					To(Equal([]byte(body)))
			})

			It("adds nothing to a body already ending in a newline", func() {
				body := "declared: yes\n"
				file := writeLocal("app.yml", body, 0o600)

				Expect(subject.CopyPath(context.Background(), file, "~/copies/app.yml")).To(Succeed())

				Expect(os.ReadFile(landed("copies/app.yml"))).
					To(Equal([]byte(body)))
			})

			It("carries content no shell could have carried", func() {
				body := string([]byte{0x00, 0xff, 0x1b, 0x0a, 0x00, 0x80})
				file := writeLocal("blob.bin", body, 0o600)

				Expect(subject.CopyPath(context.Background(), file, "~/copies/blob.bin")).
					To(Succeed())

				Expect(os.ReadFile(landed("copies/blob.bin"))).
					To(Equal([]byte(body)))
			})

			It("keeps a file executable", func() {
				file := writeLocal("run.sh", "#!/bin/sh\necho hi\n", 0o755)

				Expect(subject.CopyPath(context.Background(), file, "~/copies/run.sh")).To(Succeed())

				info, err := os.Stat(landed("copies/run.sh"))
				Expect(err).ToNot(HaveOccurred())
				Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o755)))
			})

			It("leaves an unexecutable file unexecutable", func() {
				file := writeLocal("app.yml", "declared: yes", 0o644)

				Expect(subject.CopyPath(context.Background(), file, "~/copies/app.yml")).To(Succeed())

				info, err := os.Stat(landed("copies/app.yml"))
				Expect(err).ToNot(HaveOccurred())
				Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o644)))
			})

			It("recreates a nested tree under the remote path", func() {
				writeLocal("tree/top.yml", "top", 0o600)
				writeLocal("tree/deep/inner.yml", "inner", 0o600)

				Expect(subject.CopyPath(
					context.Background(),
					filepath.Join(local, "tree"),
					"~/copies/tree",
				)).To(Succeed())

				Expect(os.ReadFile(landed("copies/tree/top.yml"))).
					To(Equal([]byte("top")))
				Expect(os.ReadFile(landed("copies/tree/deep/inner.yml"))).
					To(Equal([]byte("inner")))
			})

			It("creates a directory that holds nothing", func() {
				Expect(os.MkdirAll(
					filepath.Join(local, "empty"),
					0o700,
				)).To(Succeed())

				Expect(subject.CopyPath(
					context.Background(),
					filepath.Join(local, "empty"),
					"~/copies/empty",
				)).To(Succeed())

				entries, err := os.ReadDir(landed("copies/empty"))
				Expect(err).ToNot(HaveOccurred())
				Expect(entries).To(BeEmpty())
			})

			It("resolves a home-relative remote path", func() {
				file := writeLocal("app.yml", "declared: yes", 0o600)

				Expect(subject.CopyPath(context.Background(), file, "~/copies/app.yml")).To(Succeed())

				Expect(landed("copies/app.yml")).To(BeAnExistingFile())
				Expect(landed("~")).ToNot(BeAnExistingFile())
			})

			It("writes an absolute remote path as given", func() {
				file := writeLocal("app.yml", "declared: yes", 0o600)
				target := landed("abs/app.yml")

				Expect(subject.CopyPath(context.Background(), file, target)).To(Succeed())

				Expect(target).To(BeAnExistingFile())
			})

			It("reports a local path that is not there", func() {
				err := subject.CopyPath(
					context.Background(),
					filepath.Join(local, "nope.yml"),
					"~/copies/nope.yml",
				)

				Expect(err).To(MatchError(transport.ErrSSHLocalFileStats))
			})

			It("reports a remote path it cannot write", func() {
				file := writeLocal("app.yml", "declared: yes", 0o600)
				blocked := landed("blocked")

				Expect(os.MkdirAll(blocked, 0o500)).To(Succeed())

				Expect(subject.CopyPath(context.Background(), file, blocked+"/app.yml")).
					To(MatchError(transport.ErrSFTPRemoteWrite))
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

			return client.RunCommand(context.Background(), "true")
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

	Describe("cancelling a stalled handshake", func() {
		It("returns once the context expires", func() {
			writeKnownHosts("")

			client, err := transport.NewSSHTransport(
				transport.SSHTransportOptions{
					Config: config.XMiniEnvSSH{
						Host:     "127.0.0.1",
						Port:     silentPort(),
						User:     "tester",
						Identity: writeIdentity(GinkgoT().TempDir()),
					},
				})
			Expect(err).ToNot(HaveOccurred())

			DeferCleanup(func() { _ = client.Close() })

			ctx, cancel := context.WithTimeout(context.Background(), cancelAfter)
			defer cancel()

			err = client.RunCommand(ctx, "true")

			Expect(err).To(MatchError(transport.ErrCancelled))
			Expect(err).To(MatchError(context.DeadlineExceeded))
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

			Expect(client.RunCommand(context.Background(), "true")).
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

			Expect(client.RunCommand(context.Background(), "true")).
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

			Expect(client.RunCommand(context.Background(), "true")).
				To(MatchError(transport.ErrUserHomeDir))
		})
	})
})

func mustGetwd() string {
	GinkgoHelper()

	wd, err := os.Getwd()
	Expect(err).ToNot(HaveOccurred())

	return wd
}
