package transport_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/transport"
)

const (
	testTarget    = "i-0123456789abcdef0"
	testSessionID = "tester-0123456789abcdef0"
	testRegion    = "us-east-1"
	testToken     = "2abcTESTTOKENvalue_do_not_log"
	testBanner    = "\nStarting session with SessionId: " + testSessionID + "\n"
	testTrailer   = "Exiting session with sessionId: " + testSessionID + ".\n"
)

type fakeSessionAPI struct {
	mu         sync.Mutex
	startErr   error
	starts     []*ssm.StartSessionInput
	terminated []string
}

func (f *fakeSessionAPI) StartSession(
	_ context.Context,
	params *ssm.StartSessionInput,
	_ ...func(*ssm.Options),
) (*ssm.StartSessionOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.starts = append(f.starts, params)

	if f.startErr != nil {
		return nil, f.startErr
	}

	return &ssm.StartSessionOutput{
		SessionId:  aws.String(testSessionID),
		StreamUrl:  aws.String("wss://ssmmessages.example/" + testSessionID),
		TokenValue: aws.String(testToken),
	}, nil
}

func (f *fakeSessionAPI) TerminateSession(
	_ context.Context,
	params *ssm.TerminateSessionInput,
	_ ...func(*ssm.Options),
) (*ssm.TerminateSessionOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.terminated = append(f.terminated, aws.ToString(params.SessionId))

	return &ssm.TerminateSessionOutput{}, nil
}

func (f *fakeSessionAPI) Starts() []*ssm.StartSessionInput {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]*ssm.StartSessionInput{}, f.starts...)
}

func (f *fakeSessionAPI) Commands() []string {
	commands := []string{}

	for _, start := range f.Starts() {
		commands = append(commands, start.Parameters["command"]...)
	}

	return commands
}

func (f *fakeSessionAPI) Terminated() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string{}, f.terminated...)
}

type runner = func(
	ctx context.Context,
	inv transport.PluginInvocation,
	stdout io.Writer,
) error

// Runs the session's command in a local shell with HOME pointed at a temp
// directory, wrapped in what the plugin prints around it.
func localShell(home string) runner {
	return func(
		ctx context.Context,
		inv transport.PluginInvocation,
		stdout io.Writer,
	) error {
		_, _ = io.WriteString(stdout, testBanner)

		cmd := exec.CommandContext(
			ctx,
			"sh",
			"-c",
			inv.Input.Parameters["command"][0],
		)

		cmd.Env = append(os.Environ(), "HOME="+home)
		cmd.Stdout = stdout
		cmd.Stderr = GinkgoWriter

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("local shell: %w", err)
		}

		_, _ = io.WriteString(stdout, testTrailer)

		return nil
	}
}

func printing(output string) runner {
	return func(_ context.Context, _ transport.PluginInvocation, stdout io.Writer) error {
		_, _ = io.WriteString(stdout, output)

		return nil
	}
}

var _ = Describe("AwsSsmTransport", func() {
	Describe("validating the config", func() {
		It("rejects a missing target", func() {
			_, err := transport.NewAwsSsmTransport(
				transport.AwsSsmTransportOptions{
					Config: config.XMiniEnvAwsSsm{Profile: "dev"},
				})

			Expect(err).To(MatchError(transport.ErrInvalidExtension))
		})

		It("accepts a target on its own", func() {
			client, err := transport.NewAwsSsmTransport(
				transport.AwsSsmTransportOptions{
					Config: config.XMiniEnvAwsSsm{Target: testTarget},
				})

			Expect(err).ToNot(HaveOccurred())
			Expect(client.String()).To(Equal("AWS-SSM"))
		})
	})

	It("closes cleanly when nothing was ever run", func() {
		client, err := transport.NewAwsSsmTransport(
			transport.AwsSsmTransportOptions{
				Config: config.XMiniEnvAwsSsm{Target: testTarget},
			})

		Expect(err).ToNot(HaveOccurred())
		Expect(client.Close()).To(Succeed())
	})

	Describe("through a session", func() {
		var (
			api     *fakeSessionAPI
			home    string
			output  *bytes.Buffer
			subject *transport.AwsSsmTransport
		)

		BeforeEach(func() {
			api = &fakeSessionAPI{}
			home = GinkgoT().TempDir()
			output = &bytes.Buffer{}

			var err error

			subject, err = transport.NewAwsSsmTransport(
				transport.AwsSsmTransportOptions{
					Config: config.XMiniEnvAwsSsm{
						Target:  testTarget,
						Profile: "dev",
					},
				})
			Expect(err).ToNot(HaveOccurred())

			subject.SetSessionAPI(api, testRegion)
			subject.SetRunner(localShell(home))
			subject.SetStdout(output)
		})

		landed := func(name string) string {
			return filepath.Join(home, name)
		}

		Describe("starting the session", func() {
			It("asks for a non-interactive command session on the target", func() {
				Expect(subject.RunCommand(context.Background(), "true")).To(Succeed())

				Expect(api.Starts()).To(HaveLen(1))

				start := api.Starts()[0]

				Expect(aws.ToString(start.Target)).To(Equal(testTarget))
				Expect(aws.ToString(start.DocumentName)).
					To(Equal("AWS-StartNonInteractiveCommand"))
				Expect(start.Parameters).To(HaveKey("command"))
				Expect(start.Parameters["command"]).To(HaveLen(1))
				Expect(start.Parameters["command"][0]).To(ContainSubstring("true"))
			})

			It("hands the session and its aws context to the plugin", func() {
				var invoked []transport.PluginInvocation

				subject.SetRunner(func(
					ctx context.Context,
					inv transport.PluginInvocation,
					stdout io.Writer,
				) error {
					invoked = append(invoked, inv)

					return localShell(home)(ctx, inv, stdout)
				})

				Expect(subject.RunCommand(context.Background(), "true")).To(Succeed())

				Expect(invoked).To(HaveLen(1))
				Expect(invoked[0].Region).To(Equal(testRegion))
				Expect(invoked[0].Profile).To(Equal("dev"))
				Expect(invoked[0].Input).To(BeIdenticalTo(api.Starts()[0]))
				Expect(aws.ToString(invoked[0].Output.SessionId)).
					To(Equal(testSessionID))
			})

			It("runs one session per call", func() {
				Expect(subject.RunCommand(context.Background(), "true")).To(Succeed())
				Expect(subject.RunCommand(context.Background(), "true")).To(Succeed())

				Expect(api.Starts()).To(HaveLen(2))
			})

			It("refuses an already-cancelled context without starting one", func() {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()

				err := subject.RunCommand(ctx, "true")

				Expect(err).To(MatchError(transport.ErrCancelled))
				Expect(err).To(MatchError(context.Canceled))
				Expect(api.Starts()).To(BeEmpty())
			})

			It("reports a session that fails to start", func() {
				api.startErr = errors.New("AccessDeniedException")

				Expect(subject.RunCommand(context.Background(), "true")).
					To(MatchError(transport.ErrAwsSsmSessionStart))
			})
		})

		Describe("RunCommand", func() {
			It("returns nil on a zero exit", func() {
				Expect(subject.RunCommand(context.Background(), "true")).To(Succeed())
			})

			It("reports a non-zero exit", func() {
				err := subject.RunCommand(context.Background(), "exit 3")

				Expect(err).To(MatchError(transport.ErrAwsSsmCommandRun))
				Expect(err.Error()).To(ContainSubstring("exit status 3"))
			})

			It("reports a plain command in full", func() {
				err := subject.RunCommand(context.Background(), "docker compose up -d; false")

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("docker compose up -d; false"))
			})

			It("masks a command that could be carrying a credential", func() {
				err := subject.RunCommand(context.Background(), "cat /nonexistent/shadow")

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("cat <REDACTED>"))
				Expect(err.Error()).ToNot(ContainSubstring("shadow"))
			})

			It("forwards the command's output without the plugin's framing", func() {
				Expect(subject.RunCommand(context.Background(), "printf 'hello\\nworld\\n'")).
					To(Succeed())

				Expect(output.String()).To(Equal("hello\nworld\n"))
			})

			It("forwards output that ends without a newline", func() {
				Expect(subject.RunCommand(context.Background(), "printf 'hello'")).
					To(Succeed())

				Expect(output.String()).To(Equal("hello"))
			})

			It("keeps the status of a command that exits early", func() {
				Expect(subject.RunCommand(
					context.Background(),
					"cd /nonexistent 2>/dev/null || exit 0; echo never",
				)).To(Succeed())

				Expect(output.String()).To(BeEmpty())
			})

			It("keeps a non-zero status from an early exit", func() {
				err := subject.RunCommand(
					context.Background(),
					"status=7; exit $status",
				)

				Expect(err).To(MatchError(transport.ErrAwsSsmCommandRun))
				Expect(err.Error()).To(ContainSubstring("exit status 7"))
			})

			It("reports a session that ends without a status", func() {
				subject.SetRunner(printing(testBanner + "half an answer\n"))

				err := subject.RunCommand(context.Background(), "true")

				Expect(err).To(MatchError(transport.ErrAwsSsmCommandRun))
				Expect(err.Error()).To(ContainSubstring("without reporting a status"))
				Expect(output.String()).To(Equal("half an answer\n"))
			})

			It("reports a plugin that fails and terminates its session", func() {
				pluginErr := errors.New("plugin died")

				subject.SetRunner(func(context.Context, transport.PluginInvocation, io.Writer) error {
					return pluginErr
				})

				Expect(subject.RunCommand(context.Background(), "true")).
					To(MatchError(pluginErr))
				Expect(api.Terminated()).To(Equal([]string{testSessionID}))
			})

			It("terminates the session when a call is cancelled", func() {
				subject.SetRunner(func(ctx context.Context, _ transport.PluginInvocation, _ io.Writer) error {
					<-ctx.Done()

					return ctx.Err()
				})

				ctx, cancel := context.WithTimeout(context.Background(), cancelAfter)
				defer cancel()

				err := subject.RunCommand(ctx, "sleep")

				Expect(err).To(MatchError(transport.ErrCancelled))
				Expect(err).To(MatchError(context.DeadlineExceeded))
				Expect(api.Terminated()).To(Equal([]string{testSessionID}))
			})

			It("leaves a session that ended on its own alone", func() {
				Expect(subject.RunCommand(context.Background(), "true")).To(Succeed())

				Expect(api.Terminated()).To(BeEmpty())
			})
		})

		Describe("CreateFile", func() {
			It("creates the parent directory before writing", func() {
				Expect(subject.CreateFile(context.Background(), "~/dir/compose.yml", []byte("a"))).
					To(Succeed())

				Expect(landed("dir/compose.yml")).To(BeAnExistingFile())
			})

			It("writes the body exactly as given", func() {
				body := "services:\n  api:\n    image: repo/api:v1\n"

				Expect(subject.CreateFile(context.Background(), "~/compose.yml", []byte(body))).
					To(Succeed())

				Expect(os.ReadFile(landed("compose.yml"))).To(Equal([]byte(body)))
			})

			It("writes shell syntax as the literal text it is", func() {
				body := "GREETING=$HOME\nSUB=`id`\nOTHER=$(whoami)\nQUOTE='x'\n"

				Expect(subject.CreateFile(context.Background(), "~/.env", []byte(body))).
					To(Succeed())

				Expect(os.ReadFile(landed(".env"))).To(Equal([]byte(body)))
			})

			It("writes an empty body", func() {
				Expect(subject.CreateFile(context.Background(), "~/empty", nil)).
					To(Succeed())

				Expect(os.ReadFile(landed("empty"))).To(BeEmpty())
			})

			It("writes a path with a quote in it as given", func() {
				Expect(subject.CreateFile(context.Background(), "~/it's/compose.yml", []byte("a"))).
					To(Succeed())

				Expect(landed("it's/compose.yml")).To(BeAnExistingFile())
			})

			It("writes an absolute path as given", func() {
				target := filepath.Join(home, "abs", "compose.yml")

				Expect(subject.CreateFile(context.Background(), target, []byte("a"))).
					To(Succeed())

				Expect(target).To(BeAnExistingFile())
			})

			It("replaces a longer body rather than writing over part of it", func() {
				Expect(subject.CreateFile(context.Background(), "~/compose.yml", []byte("a long body"))).
					To(Succeed())
				Expect(subject.CreateFile(context.Background(), "~/compose.yml", []byte("short"))).
					To(Succeed())

				Expect(os.ReadFile(landed("compose.yml"))).To(Equal([]byte("short")))
			})

			It("sends a body larger than one chunk in pieces", func() {
				body := make([]byte, 100*1024)
				_, err := rand.Read(body)
				Expect(err).ToNot(HaveOccurred())

				Expect(subject.CreateFile(context.Background(), "~/blob", body)).
					To(Succeed())

				Expect(os.ReadFile(landed("blob"))).To(Equal(body))
				Expect(api.Starts()).To(HaveLen(1 + 5))
			})

			It("prints nothing for a write", func() {
				Expect(subject.CreateFile(context.Background(), "~/compose.yml", []byte("a"))).
					To(Succeed())

				Expect(output.String()).To(BeEmpty())
			})

			Describe("what a failed write reports", func() {
				const secret = "2abcTESTTOKENvalue_do_not_log"

				var blocked string

				BeforeEach(func() {
					blocked = filepath.Join(home, "blocked")

					Expect(os.MkdirAll(blocked, 0o500)).To(Succeed())
				})

				It("says nothing about the content it was writing", func() {
					err := subject.CreateFile(
						context.Background(),
						blocked+"/.env",
						[]byte("NGROK_AUTHTOKEN="+secret),
					)

					Expect(err).To(MatchError(transport.ErrAwsSsmCommandRun))
					Expect(err.Error()).ToNot(ContainSubstring(secret))
				})

				It("keeps the path of the file it was writing", func() {
					err := subject.CreateFile(context.Background(), blocked+"/.env", []byte(secret))

					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(".env"))
				})

				It("keeps the content out of every command it sends", func() {
					_ = subject.CreateFile(context.Background(), "~/.env", []byte(secret))

					for _, command := range api.Commands() {
						Expect(command).ToNot(ContainSubstring(secret))
					}
				})
			})
		})

		Describe("CopyPath", func() {
			var local string

			BeforeEach(func() {
				local = GinkgoT().TempDir()
			})

			write := func(name string, body []byte, mode os.FileMode) string {
				GinkgoHelper()

				p := filepath.Join(local, name)

				Expect(os.MkdirAll(filepath.Dir(p), 0o755)).To(Succeed())
				Expect(os.WriteFile(p, body, mode)).To(Succeed())

				return p
			}

			It("reproduces a file byte for byte", func() {
				body := []byte("line one\nline two\n")
				src := write("file.txt", body, 0o644)

				Expect(subject.CopyPath(context.Background(), src, "~/copies/file.txt")).
					To(Succeed())

				Expect(os.ReadFile(landed("copies/file.txt"))).To(Equal(body))
			})

			It("carries content no shell could have carried", func() {
				body := make([]byte, 4096)
				_, err := rand.Read(body)
				Expect(err).ToNot(HaveOccurred())

				src := write("blob", body, 0o644)

				Expect(subject.CopyPath(context.Background(), src, "~/blob")).
					To(Succeed())

				Expect(os.ReadFile(landed("blob"))).To(Equal(body))
			})

			It("keeps a file executable", func() {
				src := write("run.sh", []byte("#!/bin/sh\n"), 0o755)

				Expect(subject.CopyPath(context.Background(), src, "~/run.sh")).
					To(Succeed())

				info, err := os.Stat(landed("run.sh"))
				Expect(err).ToNot(HaveOccurred())
				Expect(info.Mode().Perm() & 0o100).ToNot(BeZero())
			})

			It("leaves an unexecutable file unexecutable", func() {
				src := write("data", []byte("x"), 0o644)

				Expect(subject.CopyPath(context.Background(), src, "~/data")).
					To(Succeed())

				info, err := os.Stat(landed("data"))
				Expect(err).ToNot(HaveOccurred())
				Expect(info.Mode().Perm() & 0o111).To(BeZero())
			})

			It("recreates a nested tree under the remote path", func() {
				write("a.txt", []byte("a"), 0o644)
				write("sub/b.txt", []byte("b"), 0o644)
				write("sub/deeper/c.txt", []byte("c"), 0o644)

				Expect(subject.CopyPath(context.Background(), local, "~/tree")).
					To(Succeed())

				Expect(os.ReadFile(landed("tree/a.txt"))).To(Equal([]byte("a")))
				Expect(os.ReadFile(landed("tree/sub/b.txt"))).To(Equal([]byte("b")))
				Expect(os.ReadFile(landed("tree/sub/deeper/c.txt"))).To(Equal([]byte("c")))
			})

			It("creates a directory that holds nothing", func() {
				Expect(os.MkdirAll(filepath.Join(local, "empty"), 0o755)).To(Succeed())

				Expect(subject.CopyPath(context.Background(), local, "~/tree")).
					To(Succeed())

				Expect(landed("tree/empty")).To(BeADirectory())
			})

			It("writes an absolute remote path as given", func() {
				src := write("file.txt", []byte("a"), 0o644)
				target := filepath.Join(home, "abs", "file.txt")

				Expect(subject.CopyPath(context.Background(), src, target)).
					To(Succeed())

				Expect(target).To(BeAnExistingFile())
			})

			It("leaves no archive behind", func() {
				src := write("file.txt", []byte("a"), 0o644)

				Expect(subject.CopyPath(context.Background(), src, "~/copies/file.txt")).
					To(Succeed())

				entries, err := os.ReadDir(landed("copies"))
				Expect(err).ToNot(HaveOccurred())
				Expect(entries).To(HaveLen(1))
			})

			It("reports a local path that is not there", func() {
				err := subject.CopyPath(
					context.Background(),
					filepath.Join(local, "missing"),
					"~/missing",
				)

				Expect(err).To(MatchError(transport.ErrAwsSsmLocalPath))
				Expect(api.Starts()).To(BeEmpty())
			})

			It("reports a remote path it cannot write", func() {
				src := write("file.txt", []byte("a"), 0o644)
				blocked := filepath.Join(home, "blocked")

				Expect(os.MkdirAll(blocked, 0o500)).To(Succeed())

				Expect(subject.CopyPath(context.Background(), src, blocked+"/file.txt")).
					To(MatchError(transport.ErrAwsSsmCommandRun))
			})
		})
	})

	Describe("the plugin process", func() {
		var output *bytes.Buffer

		BeforeEach(func() {
			output = &bytes.Buffer{}
		})

		It("streams the process's stdout", func() {
			Expect(transport.RunPlugin(
				context.Background(),
				"sh",
				[]string{"-c", "printf 'one\\ntwo\\n'"},
				nil,
				output,
			)).To(Succeed())

			Expect(output.String()).To(Equal("one\ntwo\n"))
		})

		It("passes the environment it is given", func() {
			Expect(transport.RunPlugin(
				context.Background(),
				"sh",
				[]string{"-c", "printf '%s' \"$MINIENV_TEST_VALUE\""},
				[]string{"MINIENV_TEST_VALUE=present"},
				output,
			)).To(Succeed())

			Expect(output.String()).To(Equal("present"))
		})

		// With stdin at /dev/null cat would return at once; it only waits
		// because stdin is a pipe nothing closes.
		It("holds stdin open for the life of the process", func() {
			ctx, cancel := context.WithTimeout(context.Background(), cancelAfter)
			defer cancel()

			started := time.Now()

			err := transport.RunPlugin(ctx, "cat", nil, nil, output)

			Expect(err).To(MatchError(transport.ErrCancelled))
			Expect(time.Since(started)).To(BeNumerically(">=", cancelAfter))
		})

		It("returns once the context expires", func() {
			ctx, cancel := context.WithTimeout(context.Background(), cancelAfter)
			defer cancel()

			err := transport.RunPlugin(ctx, "sleep", []string{"30"}, nil, output)

			Expect(err).To(MatchError(transport.ErrCancelled))
			Expect(err).To(MatchError(context.DeadlineExceeded))
		})

		It("reports a process that exits non-zero", func() {
			err := transport.RunPlugin(
				context.Background(),
				"sh",
				[]string{"-c", "exit 2"},
				nil,
				output,
			)

			Expect(err).To(MatchError(transport.ErrAwsSsmPluginRun))
		})

		It("reports a plugin that cannot start", func() {
			err := transport.RunPlugin(
				context.Background(),
				filepath.Join(GinkgoT().TempDir(), "missing"),
				nil,
				nil,
				output,
			)

			Expect(err).To(MatchError(transport.ErrAwsSsmPluginRun))
		})
	})

	Describe("the plugin invocation", func() {
		var (
			args []string
			env  []string
		)

		BeforeEach(func() {
			var err error

			args, env, err = transport.PluginArgs(transport.PluginInvocation{
				Region:  testRegion,
				Profile: "dev",
				Input: &ssm.StartSessionInput{
					Target:       aws.String(testTarget),
					DocumentName: aws.String("AWS-StartNonInteractiveCommand"),
					Parameters:   map[string][]string{"command": {"true"}},
					Reason:       aws.String("minienv-transport"),
				},
				Output: &ssm.StartSessionOutput{
					SessionId:  aws.String(testSessionID),
					StreamUrl:  aws.String("wss://ssmmessages.example/" + testSessionID),
					TokenValue: aws.String(testToken),
				},
			})
			Expect(err).ToNot(HaveOccurred())
		})

		It("builds the argv the aws cli hands the plugin", func() {
			Expect(args).To(HaveLen(6))
			Expect(args[0]).To(Equal("AWS_SSM_START_SESSION_RESPONSE"))
			Expect(args[1]).To(Equal(testRegion))
			Expect(args[2]).To(Equal("StartSession"))
			Expect(args[3]).To(Equal("dev"))
			Expect(args[4]).To(MatchJSON(fmt.Sprintf(`{
				"Target": %q,
				"DocumentName": "AWS-StartNonInteractiveCommand",
				"Parameters": {"command": ["true"]},
				"Reason": "minienv-transport"
			}`, testTarget)))
			Expect(args[5]).To(Equal("https://ssm.us-east-1.amazonaws.com"))
		})

		It("passes the session response through the environment", func() {
			Expect(env).To(HaveLen(1))
			Expect(env[0]).To(HavePrefix("AWS_SSM_START_SESSION_RESPONSE="))
			Expect(strings.TrimPrefix(env[0], "AWS_SSM_START_SESSION_RESPONSE=")).
				To(MatchJSON(fmt.Sprintf(`{
					"SessionId": %q,
					"StreamUrl": "wss://ssmmessages.example/%s",
					"TokenValue": %q
				}`, testSessionID, testSessionID, testToken)))
		})

		It("keeps the token out of argv", func() {
			for _, arg := range args {
				Expect(arg).ToNot(ContainSubstring(testToken))
			}
		})
	})
})
