package transport

import (
	"context"
	"io"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

type PluginInvocation = pluginInvocation

const (
	SsmWriteChunkSize = ssmWriteChunkSize
	SsmCommandMaxLen  = ssmCommandMaxLen
)

// SetSessionAPI installs a session client with the region and endpoint it
// resolved. The transport otherwise loads all three from the AWS environment
// on first use, which a test binary must never reach.
func (t *AwsSsmTransport) SetSessionAPI(
	api ssmSessionAPI,
	region string,
	endpoint string,
) {
	t.api = api
	t.region = region
	t.endpoint = endpoint
}

// ResolveEndpoint exposes endpoint resolution on its own, since the only
// production caller first loads credentials from the AWS environment.
func ResolveEndpoint(ctx context.Context, opts ssm.Options) (string, error) {
	return resolveEndpoint(ctx, opts)
}

// SetRunner replaces the plugin spawn with something the spec supplies — a
// local shell, usually — so the scripts the transport writes run for real.
func (t *AwsSsmTransport) SetRunner(
	run func(
		ctx context.Context,
		inv PluginInvocation,
		stdout io.Writer,
	) error,
) {
	t.run = run
}

// SetStdout redirects what the remote command printed, which otherwise goes
// to the process's stdout.
func (t *AwsSsmTransport) SetStdout(w io.Writer) {
	t.stdout = w
}

// RunPlugin exposes the process-backed session with the binary substitutable,
// so a spec can stand a coreutil in for session-manager-plugin.
func RunPlugin(
	ctx context.Context,
	binary string,
	args []string,
	env []string,
	stdout io.Writer,
) error {
	return runPlugin(ctx, binary, args, env, stdout)
}

// PluginArgs exposes the argv and environment the plugin is started with.
func PluginArgs(inv PluginInvocation) ([]string, []string, error) {
	return inv.pluginArgs()
}
