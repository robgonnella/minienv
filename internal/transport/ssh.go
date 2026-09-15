package transport

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/errs"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const (
	// An unset Timeout means the dial has no deadline at all, and ssh.Dial
	// takes no context, so a black-holing host would hang with nothing to
	// cancel it.
	sshDialTimeout = 30 * time.Second
)

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
			// Ensures credentials can't leak downstream when logging an error.
			maskCommand(cmd),
			err,
		)
	}

	return nil
}

type SSHTransportOptions struct {
	Config config.XMiniEnvSSH
}

type SSHTransport struct {
	config     config.XMiniEnvSSH
	client     *ssh.Client
	sftpClient *sftp.Client
}

func NewSSHTransport(opts SSHTransportOptions) (*SSHTransport, error) {
	if opts.Config.Host == "" || opts.Config.Identity == "" {
		return nil, errs.Errorf(
			ErrInvalidExtension,
			"missing one or both of required fields in ssh transport config: "+
				"[host, identity]",
		)
	}

	return &SSHTransport{
		config:     opts.Config,
		client:     nil,
		sftpClient: nil,
	}, nil
}

func (s *SSHTransport) String() string {
	return "SSH"
}

// Close shuts the sftp client first: it rides on the ssh connection.
func (s *SSHTransport) Close() error {
	sftpClient := s.sftpClient
	s.sftpClient = nil

	var closeErr error

	if sftpClient != nil {
		if err := sftpClient.Close(); err != nil {
			closeErr = errors.Join(closeErr, errs.Errorf(
				ErrSFTPClientClose,
				"failed to close sftp client: %w",
				err,
			))
		}
	}

	if s.client == nil {
		return closeErr
	}

	client := s.client
	s.client = nil

	if err := client.Close(); err != nil {
		closeErr = errors.Join(closeErr, errs.Errorf(
			ErrSSHClientClose,
			"failed to close ssh client: %w",
			err,
		))
	}

	return closeErr
}

// CreateFile writes content to file, replacing whatever was there.
func (s *SSHTransport) CreateFile(
	file string,
	content []byte,
) error {
	client, err := s.sftp()
	if err != nil {
		return err
	}

	target, err := resolveRemotePath(client, file)
	if err != nil {
		return err
	}

	if err := makeRemoteParent(client, target); err != nil {
		return err
	}

	if err := writeRemoteFile(
		client,
		bytes.NewReader(content),
		target,
	); err != nil {
		return err
	}

	log.
		Info().
		Str("file", target).
		Msg("created file on remote server")

	return nil
}

// CopyPath reproduces localPath at remotePath byte for byte, permissions
// included, recursing when it names a directory.
func (s *SSHTransport) CopyPath(localPath, remotePath string) error {
	info, err := os.Stat(localPath)
	if err != nil {
		return errs.Errorf(
			ErrSSHLocalFileStats,
			"failed to get information on file: %w",
			err,
		)
	}

	client, err := s.sftp()
	if err != nil {
		return err
	}

	target, err := resolveRemotePath(client, remotePath)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return s.copyTree(client, localPath, target)
	}

	return copyFile(client, localPath, target, info.Mode())
}

func (s *SSHTransport) RunCommand(cmd string) error {
	client, err := s.connect()
	if err != nil {
		return err
	}

	return runRemoteCommand(client, cmd)
}

func (s *SSHTransport) sftp() (*sftp.Client, error) {
	if s.sftpClient != nil {
		return s.sftpClient, nil
	}

	client, err := s.connect()
	if err != nil {
		return nil, err
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return nil, errs.Errorf(
			ErrSFTPClientCreate,
			"failed to open an sftp session: %w",
			err,
		)
	}

	s.sftpClient = sftpClient

	return sftpClient, nil
}

// There is no shell behind sftp to expand a leading "~".
func resolveRemotePath(client *sftp.Client, remotePath string) (string, error) {
	if remotePath != "~" && !strings.HasPrefix(remotePath, "~/") {
		return remotePath, nil
	}

	home, err := client.RealPath(".")
	if err != nil {
		return "", errs.Errorf(
			ErrSFTPRealPath,
			"failed to resolve the remote home directory: %w",
			err,
		)
	}

	return path.Join(home, strings.TrimPrefix(remotePath, "~")), nil
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

func (s *SSHTransport) copyTree(
	client *sftp.Client,
	localDir string,
	remoteDir string,
) error {
	walk := func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return errs.Errorf(
				ErrSSHWalkLocalPath,
				"failed to walk %s: %w",
				current,
				err,
			)
		}

		target, err := remoteJoin(localDir, remoteDir, current)
		if err != nil {
			return err
		}

		info, err := entry.Info()
		if err != nil {
			return errs.Errorf(
				ErrSSHWalkLocalPath,
				"failed to get information on %s: %w",
				current,
				err,
			)
		}

		if entry.IsDir() {
			return makeRemoteDir(client, target, info.Mode())
		}

		return copyFile(client, current, target, info.Mode())
	}

	if err := filepath.WalkDir(localDir, walk); err != nil {
		return err
	}

	return nil
}

func remoteJoin(localDir, remoteDir, current string) (string, error) {
	rel, err := filepath.Rel(localDir, current)
	if err != nil {
		return "", errs.Errorf(
			ErrSSHWalkLocalPath,
			"failed to place %s under %s: %w",
			current,
			localDir,
			err,
		)
	}

	if rel == "." {
		return remoteDir, nil
	}

	return path.Join(remoteDir, filepath.ToSlash(rel)), nil
}

func makeRemoteDir(
	client *sftp.Client,
	remotePath string,
	mode fs.FileMode,
) error {
	if err := client.MkdirAll(remotePath); err != nil {
		return errs.Errorf(
			ErrSFTPRemoteWrite,
			"failed to create remote directory %s: %w",
			remotePath,
			err,
		)
	}

	return chmodRemote(client, remotePath, mode)
}

func copyFile(
	client *sftp.Client,
	localPath string,
	remotePath string,
	mode fs.FileMode,
) error {
	// #nosec G304 -- validated as a project-relative path in config.
	src, err := os.Open(localPath)
	if err != nil {
		return errs.Errorf(
			ErrSSHReadLocalFile,
			"failed to read file %s: %w",
			localPath,
			err,
		)
	}

	defer func() {
		if err := src.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close local file")
		}
	}()

	if err := makeRemoteParent(client, remotePath); err != nil {
		return err
	}

	if err := writeRemoteFile(client, src, remotePath); err != nil {
		return err
	}

	log.
		Info().
		Str("src", src.Name()).
		Str("dest", remotePath).
		Msg("copied file to remote destination")

	return chmodRemote(client, remotePath, mode)
}

func makeRemoteParent(client *sftp.Client, remotePath string) error {
	if err := client.MkdirAll(path.Dir(remotePath)); err != nil {
		return errs.Errorf(
			ErrSFTPRemoteWrite,
			"failed to create remote directory %s: %w",
			path.Dir(remotePath),
			err,
		)
	}

	return nil
}

// Close is checked rather than deferred: it is where a failed write surfaces.
func writeRemoteFile(
	client *sftp.Client,
	src io.Reader,
	remotePath string,
) error {
	dst, err := client.Create(remotePath)
	if err != nil {
		return errs.Errorf(
			ErrSFTPRemoteWrite,
			"failed to create remote file %s: %w",
			remotePath,
			err,
		)
	}

	if _, err := io.Copy(dst, src); err != nil {
		if err := dst.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close remote file")
		}

		return errs.Errorf(
			ErrSFTPRemoteWrite,
			"failed to write remote file %s: %w",
			remotePath,
			err,
		)
	}

	if err := dst.Close(); err != nil {
		return errs.Errorf(
			ErrSFTPRemoteWrite,
			"failed to close remote file %s: %w",
			remotePath,
			err,
		)
	}

	return nil
}

func chmodRemote(
	client *sftp.Client,
	remotePath string,
	mode fs.FileMode,
) error {
	if err := client.Chmod(remotePath, mode.Perm()); err != nil {
		return errs.Errorf(
			ErrSFTPRemoteWrite,
			"failed to set the mode on %s: %w",
			remotePath,
			err,
		)
	}

	return nil
}
