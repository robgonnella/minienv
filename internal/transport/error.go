// Package transport is the contract for reaching a remote host, and the
// implementations that carry files and commands to it.
package transport

import "github.com/robgonnella/minienv/internal/errs"

// Failure modes raised by this package. Values are namespaced because errs.Kind
// is one shared type — see internal/errs.
const (
	ErrInvalidExtension    errs.Kind = "transport.invalid_extension"
	ErrSSHTransportUser    errs.Kind = "transport.ssh_transport_user"
	ErrSSHIdentityResolve  errs.Kind = "transport.ssh_identity_resolve"
	ErrSSHIdentityRead     errs.Kind = "transport.ssh_identity_read"
	ErrSSHSignerCreate     errs.Kind = "transport.ssh_signer_create"
	ErrSSHClientCreate     errs.Kind = "transport.ssh_client_create"
	ErrSSHClientClose      errs.Kind = "transport.ssh_client_close"
	ErrSSHSessionCreate    errs.Kind = "transport.ssh_session_create"
	ErrSSHCommandRun       errs.Kind = "transport.ssh_command_run"
	ErrSSHUserHomeDir      errs.Kind = "transport.ssh_user_home_dir"
	ErrSSHHostKeyCallback  errs.Kind = "transport.ssh_hostkey_callback"
	ErrSSHHeredocDelimiter errs.Kind = "transport.ssh_heredoc_delimiter"
)
