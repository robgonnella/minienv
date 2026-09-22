package transport_test

import (
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Length prefix on an "exec" request payload, per RFC 4254 section 6.5.
const execPayloadPrefixLen = 4

const identityFileMode = 0o600

const sshDirMode = 0o700

// An ssh server in this process, on loopback.
type testServer struct {
	listener    net.Listener
	hostKey     ssh.PublicKey
	mu          sync.Mutex
	commands    []string
	exitCode    int
	refuseAll   bool
	hang        bool
	closed      bool
	sftpHome    string
	release     chan struct{}
	releaseOnce sync.Once
	wg          sync.WaitGroup
}

// Registers its own teardown: --randomize-all means a leaked accept goroutine
// would surface in an unrelated spec.
func newTestServer() *testServer {
	GinkgoHelper()

	hostPub, hostPriv, err := ed25519.GenerateKey(nil)
	Expect(err).ToNot(HaveOccurred())

	hostSigner, err := ssh.NewSignerFromKey(hostPriv)
	Expect(err).ToNot(HaveOccurred())

	sshHostPub, err := ssh.NewPublicKey(hostPub)
	Expect(err).ToNot(HaveOccurred())

	var lc net.ListenConfig

	listener, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	Expect(err).ToNot(HaveOccurred())

	srv := &testServer{
		listener: listener,
		hostKey:  sshHostPub,
		sftpHome: GinkgoT().TempDir(),
		release:  make(chan struct{}),
	}

	config := &ssh.ServerConfig{
		PublicKeyCallback: func(
			ssh.ConnMetadata,
			ssh.PublicKey,
		) (*ssh.Permissions, error) {
			return &ssh.Permissions{}, nil
		},
	}
	config.AddHostKey(hostSigner)

	srv.wg.Add(1)

	go srv.accept(config)

	DeferCleanup(srv.stop)

	return srv
}

func (s *testServer) FailWith(code int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.exitCode = code
}

// Accepts the connection but rejects every session channel, which is the only
// way to reach ErrSSHSessionCreate.
func (s *testServer) RefuseSessions() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.refuseAll = true
}

// Hang accepts every exec and never reports an exit status, so the client
// stays blocked in the command until it gives up or Release is called.
func (s *testServer) Hang() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.hang = true
}

func (s *testServer) Release() {
	s.mu.Lock()
	s.hang = false
	s.mu.Unlock()

	s.releaseOnce.Do(func() { close(s.release) })
}

func (s *testServer) Commands() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]string{}, s.commands...)
}

func (s *testServer) Port() uint16 {
	GinkgoHelper()

	_, port, err := net.SplitHostPort(s.listener.Addr().String())
	Expect(err).ToNot(HaveOccurred())

	parsed, err := strconv.ParseUint(port, 10, 16)
	Expect(err).ToNot(HaveOccurred())

	return uint16(parsed)
}

func (s *testServer) HostKey() ssh.PublicKey {
	return s.hostKey
}

// SFTPHome is the directory the server reports for ".", so a remote path
// written as "~/..." resolves inside it.
func (s *testServer) SFTPHome() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.sftpHome
}

func (s *testServer) accept(config *ssh.ServerConfig) {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}

		s.wg.Add(1)

		go s.serve(conn, config)
	}
}

func (s *testServer) serve(conn net.Conn, config *ssh.ServerConfig) {
	defer s.wg.Done()
	defer func() { _ = conn.Close() }()

	_, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}

	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {
		s.handleChannel(newChannel)
	}
}

func (s *testServer) handleChannel(newChannel ssh.NewChannel) {
	if newChannel.ChannelType() != "session" || s.refusing() {
		_ = newChannel.Reject(ssh.Prohibited, "refused")
		return
	}

	channel, requests, err := newChannel.Accept()
	if err != nil {
		return
	}

	defer func() { _ = channel.Close() }()

	for req := range requests {
		if req.Type == "subsystem" && s.isSFTP(req.Payload) {
			_ = req.Reply(true, nil)

			s.serveSFTP(channel)

			return
		}

		if req.Type != "exec" {
			_ = req.Reply(false, nil)
			continue
		}

		s.recordExec(req.Payload)

		_ = req.Reply(true, nil)

		if s.hanging() {
			<-s.release

			return
		}

		status := make([]byte, execPayloadPrefixLen)
		binary.BigEndian.PutUint32(status, uint32(s.exit()))
		_, _ = channel.SendRequest("exit-status", false, status)

		return
	}
}

// A subsystem payload carries its name the same way an exec payload does.
func (s *testServer) isSFTP(payload []byte) bool {
	if len(payload) < execPayloadPrefixLen {
		return false
	}

	return string(payload[execPayloadPrefixLen:]) == "sftp"
}

// Serves the real filesystem, rooted for "." at a temp directory, so specs can
// assert on the bytes and modes that land.
func (s *testServer) serveSFTP(channel ssh.Channel) {
	server, err := sftp.NewServer(
		channel,
		sftp.WithServerWorkingDirectory(s.SFTPHome()),
	)
	if err != nil {
		return
	}

	defer func() { _ = server.Close() }()

	_ = server.Serve()
}

// An exec payload is a 4-byte big-endian length followed by the command.
func (s *testServer) recordExec(payload []byte) {
	if len(payload) < execPayloadPrefixLen {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.commands = append(s.commands, string(payload[execPayloadPrefixLen:]))
}

func (s *testServer) stop() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}

	s.closed = true
	s.mu.Unlock()

	s.Release()

	_ = s.listener.Close()
	s.wg.Wait()
}

func (s *testServer) hanging() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.hang
}

// A listening socket that is never accepted from: the TCP connect completes
// from the backlog, and the client then waits forever for a server banner.
func silentPort() uint16 {
	GinkgoHelper()

	var lc net.ListenConfig

	listener, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	Expect(err).ToNot(HaveOccurred())

	DeferCleanup(func() { _ = listener.Close() })

	return uint16(listener.Addr().(*net.TCPAddr).Port)
}

func (s *testServer) exit() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.exitCode
}

func (s *testServer) refusing() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.refuseAll
}

// ed25519 rather than RSA: keygen runs once per spec and RSA-2048 is slow
// enough to be felt across the suite.
func writeIdentity(dir string) string {
	GinkgoHelper()

	_, priv, err := ed25519.GenerateKey(nil)
	Expect(err).ToNot(HaveOccurred())

	block, err := ssh.MarshalPrivateKey(priv, "")
	Expect(err).ToNot(HaveOccurred())

	path := filepath.Join(dir, "id_ed25519")
	Expect(os.WriteFile(path, pem.EncodeToMemory(block), identityFileMode)).
		To(Succeed())

	return path
}

// knownhosts.Line rather than a hand-built entry: the server listens on an
// ephemeral port, and a non-22 host is recorded as "[127.0.0.1]:<port>".
func trustHost(srv *testServer) string {
	GinkgoHelper()

	return writeKnownHosts(knownhosts.Line(
		[]string{knownhosts.Normalize(
			net.JoinHostPort("127.0.0.1", strconv.Itoa(int(srv.Port()))),
		)},
		srv.HostKey(),
	))
}

// os.UserHomeDir reads $HOME, so redirecting it is enough to retarget the
// host-key check without a seam in the production code.
func writeKnownHosts(line string) string {
	GinkgoHelper()

	home := GinkgoT().TempDir()
	sshDir := filepath.Join(home, ".ssh")

	Expect(os.MkdirAll(sshDir, sshDirMode)).To(Succeed())
	Expect(os.WriteFile(
		filepath.Join(sshDir, "known_hosts"),
		[]byte(line+"\n"),
		identityFileMode,
	)).To(Succeed())

	GinkgoT().Setenv("HOME", home)

	return home
}
