package transport

import (
	"context"
	"io"
)

type PluginInvocation = pluginInvocation

// SetSessionAPI installs a session client and the region it resolved in. The
// transport otherwise loads both from the AWS environment on first use,
// which a test binary must never reach.
func (t *AwsSsmTransport) SetSessionAPI(api ssmSessionAPI, region string) {
	t.api = api
	t.region = region
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
