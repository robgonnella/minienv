// Package transport is the contract for reaching a remote host, and the
// implementations that carry files and commands to it.
package transport

import "github.com/robgonnella/minienv/internal/errs"

const (
	ErrInvalidExtension   errs.Kind = "transport.invalid_extension"
	ErrCancelled          errs.Kind = "transport.cancelled"
	ErrUserHomeDir        errs.Kind = "transport.user_home_dir"
	ErrSSHTransportUser   errs.Kind = "transport.ssh_transport_user"
	ErrSSHIdentityResolve errs.Kind = "transport.ssh_identity_resolve"
	ErrSSHIdentityRead    errs.Kind = "transport.ssh_identity_read"
	ErrSSHSignerCreate    errs.Kind = "transport.ssh_signer_create"
	ErrSSHClientCreate    errs.Kind = "transport.ssh_client_create"
	ErrSSHClientClose     errs.Kind = "transport.ssh_client_close"
	ErrSSHSessionCreate   errs.Kind = "transport.ssh_session_create"
	ErrSSHCommandRun      errs.Kind = "transport.ssh_command_run"
	ErrSSHHostKeyCallback errs.Kind = "transport.ssh_hostkey_callback"
	ErrSSHLocalFileStats  errs.Kind = "transport.ssh_local_file_stats"
	ErrSSHWalkLocalPath   errs.Kind = "transport.ssh_walk_local_path"
	ErrSSHReadLocalFile   errs.Kind = "transport.ssh_read_local_file"
	ErrSFTPClientCreate   errs.Kind = "transport.sftp_client_create"
	ErrSFTPClientClose    errs.Kind = "transport.sftp_client_close"
	ErrSFTPRealPath       errs.Kind = "transport.sftp_real_path"
	ErrSFTPRemoteWrite    errs.Kind = "transport.sftp_remote_write"
)
