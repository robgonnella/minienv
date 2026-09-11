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
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Length prefix on an "exec" request payload, per RFC 4254 section 6.5.
const execPayloadPrefixLen = 4

const identityFileMode = 0o600

const sshDirMode = 0o700

// An ssh server in this process, on loopback. The heredoc quoting under test
// is only observable in what a server actually receives.
type testServer struct {
	listener net.Listener
	hostKey  ssh.PublicKey

	mu        sync.Mutex
	commands  []string
	exitCode  int
	refuseAll bool
	closed    bool

	wg sync.WaitGroup
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
		if req.Type != "exec" {
			_ = req.Reply(false, nil)
			continue
		}

		s.recordExec(req.Payload)

		_ = req.Reply(true, nil)

		status := make([]byte, execPayloadPrefixLen)
		binary.BigEndian.PutUint32(status, uint32(s.exit()))
		_, _ = channel.SendRequest("exit-status", false, status)

		return
	}
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

	_ = s.listener.Close()
	s.wg.Wait()
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
