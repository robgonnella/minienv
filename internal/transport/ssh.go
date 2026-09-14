package transport

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"time"

	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/errs"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const (
	heredocNonceBytes = 8
	// An unset Timeout means the dial has no deadline at all, and ssh.Dial
	// takes no context, so a black-holing host would hang with nothing to
	// cancel it.
	sshDialTimeout = 30 * time.Second
)

// Masks cmd before reporting it because a command body can carry a credential,
// and this error is logged in full further up.
func runRemoteCommand(client *ssh.Client, cmd string) error {
	session, err := client.NewSession()
	if err != nil {
		return errs.Errorf(
			ErrSSHSessionCreate,
			"failed to create session: %w",
			err,
		)
	}
	// A session closes itself once the command returns, so the deferred Close
	// is only there for the early-return paths and io.EOF is expected.
	defer func() {
		if err := session.Close(); err != nil && !errors.Is(err, io.EOF) {
			log.Warn().Err(err).Msg("failed to close ssh session")
		}
	}()

	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	if err := session.Run(cmd); err != nil {
		return errs.Errorf(
			ErrSSHCommandRun,
			"remote command failed: %s: %w",
			maskCommand(cmd),
			err,
		)
	}

	return nil
}

// A fixed "EOF" truncates any file containing a bare EOF line.
func heredocDelimiter() (string, error) {
	nonce := make([]byte, heredocNonceBytes)
	if _, err := rand.Read(nonce); err != nil {
		return "", errs.Errorf(
			ErrSSHHeredocDelimiter,
			"failed to generate heredoc delimiter: %w",
			err,
		)
	}

	return "MINIENV_" + hex.EncodeToString(nonce), nil
}

type SSHTransportOptions struct {
	Config config.XMiniEnvSSH
}

type SSHTransport struct {
	config config.XMiniEnvSSH
	client *ssh.Client
}

func NewSSHTransport(opts SSHTransportOptions) (*SSHTransport, error) {
	if opts.Config.Host == "" || opts.Config.Identity == "" {
		return nil, errs.Errorf(
			ErrInvalidExtension,
			"missing one or both of required fields in ssh transport config: "+
				"[host, identity]",
		)
	}

	return &SSHTransport{config: opts.Config, client: nil}, nil
}

func (s *SSHTransport) String() string {
	return "SSH"
}

func (s *SSHTransport) Close() error {
	if s.client == nil {
		return nil
	}

	client := s.client
	s.client = nil

	if err := client.Close(); err != nil {
		return errs.Errorf(
			ErrSSHClientClose,
			"failed to close ssh client: %w",
			err,
		)
	}

	return nil
}

func (s *SSHTransport) CreateFile(
	file string,
	content []byte,
) error {
	client, err := s.connect()
	if err != nil {
		return err
	}

	delimiter, err := heredocDelimiter()
	if err != nil {
		return err
	}

	dir := path.Dir(file)

	// Quoting the delimiter is what stops the remote shell expanding the body.
	cmd := fmt.Sprintf(`mkdir -p %s && cat << '%s' > %s
%s
%s`, dir, delimiter, file, content, delimiter)

	if err := runRemoteCommand(client, cmd); err != nil {
		return err
	}

	log.
		Info().
		Str("file", file).
		Msg("Successfully created file on remote server")

	return nil
}

func (s *SSHTransport) RunCommand(cmd string) error {
	client, err := s.connect()
	if err != nil {
		return err
	}

	return runRemoteCommand(client, cmd)
}

func (s *SSHTransport) connect() (*ssh.Client, error) {
	if s.client != nil {
		return s.client, nil
	}

	client, err := s.createClient()
	if err != nil {
		return nil, err
	}

	s.client = client

	return client, nil
}

func (s *SSHTransport) identitySigner() (ssh.Signer, error) {
	identity, err := expandHome(s.config.Identity)
	if err != nil {
		return nil, err
	}

	privKeyPath, err := filepath.Abs(identity)
	if err != nil {
		return nil, errs.Errorf(
			ErrSSHIdentityResolve,
			"could not resolve identity path: %s: %w",
			s.config.Identity,
			err,
		)
	}

	// #nosec G304 -- the identity path is the project's own ssh key, named in
	// its compose config; reading whatever it points at is the whole point.
	privKeyBytes, err := os.ReadFile(privKeyPath)
	if err != nil {
		return nil, errs.Errorf(
			ErrSSHIdentityRead,
			"failed to read public key file: %s: %w",
			s.config.Identity,
			err,
		)
	}

	signer, err := ssh.ParsePrivateKey(privKeyBytes)
	if err != nil {
		return nil, errs.Errorf(
			ErrSSHSignerCreate,
			"failed to create signer from identity: %w",
			err,
		)
	}

	return signer, nil
}

func (s *SSHTransport) createClient() (*ssh.Client, error) {
	username, err := s.username()
	if err != nil {
		return nil, err
	}

	port := s.config.Port
	if port == 0 {
		port = 22
	}

	signer, err := s.identitySigner()
	if err != nil {
		return nil, err
	}

	hostKeyCallback, err := s.knownHostkeyCallback()
	if err != nil {
		return nil, err
	}

	sshConfig := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: hostKeyCallback,
		Timeout:         sshDialTimeout,
	}

	addr := fmt.Sprintf("%s:%d", s.config.Host, port)

	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return nil, errs.Errorf(
			ErrSSHClientCreate,
			"failed to create ssh client: %w",
			err,
		)
	}

	return client, nil
}

func (s *SSHTransport) username() (string, error) {
	username := s.config.User
	if username == "" {
		currentUser, err := user.Current()
		if err != nil {
			return "", errs.Errorf(
				ErrSSHTransportUser,
				"failed to look up current user: %w",
				err,
			)
		}

		username = currentUser.Username
	}

	return username, nil
}

func (s *SSHTransport) knownHostkeyCallback() (ssh.HostKeyCallback, error) {
	knownHostsPath, err := expandHome("~/.ssh/known_hosts")
	if err != nil {
		return nil, err
	}

	hostKeyCallback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return nil, errs.Errorf(
			ErrSSHHostKeyCallback,
			"failed to create known hosts callback: %w",
			err,
		)
	}

	return hostKeyCallback, nil
}
