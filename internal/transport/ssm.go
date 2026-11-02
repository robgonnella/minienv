package transport

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/robgonnella/minienv/internal/config"
	"github.com/robgonnella/minienv/internal/errs"
	"github.com/rs/zerolog/log"
)

const (
	ssmSessionDocument   = "AWS-StartNonInteractiveCommand"
	ssmSessionReason     = "minienv-transport"
	ssmPluginBinary      = "session-manager-plugin"
	ssmPluginOperation   = "StartSession"
	ssmPluginResponseEnv = "AWS_SSM_START_SESSION_RESPONSE"
	ssmPluginBanner      = "Starting session with SessionId: "
	ssmPluginTrailer     = "Exiting session with sessionId: "
	ssmStatusMarker      = "__minienv_status_"
	ssmStatusMarkerBytes = 8
	ssmTeardownTimeout   = 10 * time.Second
	ssmWriteChunkSize    = 24 * 1024
	ssmArchiveSuffix     = ".minienv.tgz"
)

type ssmSessionAPI interface {
	StartSession(
		ctx context.Context,
		params *ssm.StartSessionInput,
		optFns ...func(*ssm.Options),
	) (*ssm.StartSessionOutput, error)
	TerminateSession(
		ctx context.Context,
		params *ssm.TerminateSessionInput,
		optFns ...func(*ssm.Options),
	) (*ssm.TerminateSessionOutput, error)
}

type pluginInvocation struct {
	Region  string
	Profile string
	Input   *ssm.StartSessionInput
	Output  *ssm.StartSessionOutput
}

type runSessionFunc func(
	ctx context.Context,
	inv pluginInvocation,
	stdout io.Writer,
) error

type AwsSsmTransportOptions struct {
	Config config.XMiniEnvAwsSsm
}

type AwsSsmTransport struct {
	config config.XMiniEnvAwsSsm
	api    ssmSessionAPI
	region string
	run    runSessionFunc
	stdout io.Writer
}

func NewAwsSsmTransport(
	opts AwsSsmTransportOptions,
) (*AwsSsmTransport, error) {
	if opts.Config.Target == "" {
		return nil, errs.Errorf(
			ErrInvalidExtension,
			"missing required field in awsSsm transport config: [target]",
		)
	}

	return &AwsSsmTransport{
		config: opts.Config,
		api:    nil,
		region: "",
		run:    runPluginSession,
		stdout: os.Stdout,
	}, nil
}

func (t *AwsSsmTransport) String() string {
	return "AWS-SSM"
}

func (t *AwsSsmTransport) Close() error {
	return nil
}

func (t *AwsSsmTransport) RunCommand(ctx context.Context, cmd string) error {
	return t.runScript(ctx, cmd, maskCommand(cmd))
}

func (t *AwsSsmTransport) CreateFile(
	ctx context.Context,
	file string,
	content []byte,
) error {
	if err := t.writeRemote(ctx, file, content); err != nil {
		return err
	}

	log.
		Info().
		Str("file", file).
		Msg("created file on remote server")

	return nil
}

func (t *AwsSsmTransport) CopyPath(
	ctx context.Context,
	localPath string,
	remotePath string,
) error {
	info, err := os.Stat(localPath)
	if err != nil {
		return errs.Errorf(
			ErrAwsSsmLocalPath,
			"failed to get information on file: %w",
			err,
		)
	}

	archive, err := archivePath(localPath, path.Base(remotePath), info)
	if err != nil {
		return err
	}

	archivePath := remotePath + ssmArchiveSuffix

	if err := t.writeRemote(ctx, archivePath, archive); err != nil {
		return err
	}

	extractDir := path.Dir(remotePath)
	script := "if ! command -v tar >/dev/null || " +
		"! command -v gzip >/dev/null; then " +
		"echo 'tar and gzip are needed on the instance' >&2; " +
		"rm -f " + shellPath(archivePath) + "; " +
		"exit 127; fi; "

	if info.IsDir() {
		extractDir = remotePath
		script += "mkdir -p " + shellPath(remotePath) + " && "
	}

	script += "tar -xzpf " + shellPath(archivePath) +
		" -C " + shellPath(extractDir) + "; " +
		"__minienv_rc=$?; " +
		"rm -f " + shellPath(archivePath) + "; " +
		"exit $__minienv_rc"

	if err := t.runScript(
		ctx,
		script,
		"copy "+localPath+" to "+remotePath,
	); err != nil {
		return err
	}

	log.
		Info().
		Str("src", localPath).
		Str("dest", remotePath).
		Msg("copied path to remote destination")

	return nil
}

func (t *AwsSsmTransport) writeRemote(
	ctx context.Context,
	remotePath string,
	content []byte,
) error {
	target := shellPath(remotePath)
	describe := "write " + remotePath

	if err := t.runScript(
		ctx,
		"mkdir -p "+shellPath(path.Dir(remotePath))+" && : > "+target,
		describe,
	); err != nil {
		return err
	}

	for offset := 0; offset < len(content); offset += ssmWriteChunkSize {
		end := min(offset+ssmWriteChunkSize, len(content))
		chunk := base64.StdEncoding.EncodeToString(content[offset:end])

		if err := t.runScript(
			ctx,
			"printf '%s' '"+chunk+"' | base64 -d >> "+target,
			describe,
		); err != nil {
			return err
		}
	}

	return nil
}

func (t *AwsSsmTransport) runScript(
	ctx context.Context,
	script string,
	describe string,
) error {
	if err := ctx.Err(); err != nil {
		return cancelled(err)
	}

	marker, err := newStatusMarker()
	if err != nil {
		return err
	}

	inv, err := t.startSession(
		ctx,
		"(\n"+script+"\n)\nprintf '%s=%s\\n' '"+marker+"' \"$?\"\n",
	)
	if err != nil {
		return err
	}

	filter := &statusFilter{dest: t.stdout, marker: marker}
	runErr := t.run(ctx, inv, filter)

	if err := filter.Flush(); err != nil {
		return err
	}

	if runErr != nil {
		t.terminateSession(ctx, aws.ToString(inv.Output.SessionId))

		if ctxErr := ctx.Err(); ctxErr != nil {
			return cancelled(ctxErr)
		}

		return runErr
	}

	code, ok := filter.Status()

	switch {
	case !ok:
		return errs.Errorf(
			ErrAwsSsmCommandRun,
			"remote command ended without reporting a status: %s",
			describe,
		)
	case code != 0:
		return errs.Errorf(
			ErrAwsSsmCommandRun,
			"remote command failed: %s: exit status %d",
			describe,
			code,
		)
	default:
		return nil
	}
}

func (t *AwsSsmTransport) startSession(
	ctx context.Context,
	command string,
) (pluginInvocation, error) {
	api, err := t.sessionAPI(ctx)
	if err != nil {
		return pluginInvocation{}, err
	}

	input := &ssm.StartSessionInput{
		Target:       aws.String(t.config.Target),
		DocumentName: aws.String(ssmSessionDocument),
		Parameters:   map[string][]string{"command": {command}},
		Reason:       aws.String(ssmSessionReason),
	}

	output, err := api.StartSession(ctx, input)
	if err != nil {
		return pluginInvocation{}, errs.Errorf(
			ErrAwsSsmSessionStart,
			"failed to start aws-ssm session on %s: %w",
			t.config.Target,
			err,
		)
	}

	log.
		Debug().
		Str("target", t.config.Target).
		Str("session", aws.ToString(output.SessionId)).
		Msg("started aws-ssm session")

	return pluginInvocation{
		Region:  t.region,
		Profile: t.config.Profile,
		Input:   input,
		Output:  output,
	}, nil
}

func (t *AwsSsmTransport) sessionAPI(
	ctx context.Context,
) (ssmSessionAPI, error) {
	if t.api != nil {
		return t.api, nil
	}

	loadOpts := []func(*awsconfig.LoadOptions) error{}

	if t.config.Profile != "" {
		loadOpts = append(
			loadOpts,
			awsconfig.WithSharedConfigProfile(t.config.Profile),
		)
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, errs.Errorf(
			ErrAwsSsmConfigure,
			"failed to load aws configuration: %w",
			err,
		)
	}

	if cfg.Region == "" {
		return nil, errs.Errorf(
			ErrAwsSsmConfigure,
			"no aws region resolved: set AWS_REGION or a region on the profile",
		)
	}

	t.region = cfg.Region
	t.api = ssm.NewFromConfig(cfg)

	return t.api, nil
}

func (t *AwsSsmTransport) terminateSession(
	ctx context.Context,
	sessionID string,
) {
	ctx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		ssmTeardownTimeout,
	)
	defer cancel()

	if _, err := t.api.TerminateSession(ctx, &ssm.TerminateSessionInput{
		SessionId: aws.String(sessionID),
	}); err != nil {
		log.
			Warn().
			Err(err).
			Str("session", sessionID).
			Msg("failed to terminate aws-ssm session")

		return
	}

	log.
		Debug().
		Str("session", sessionID).
		Msg("terminated aws-ssm session")
}

func newStatusMarker() (string, error) {
	random := make([]byte, ssmStatusMarkerBytes)

	if _, err := rand.Read(random); err != nil {
		return "", errs.Errorf(
			ErrAwsSsmCommandRun,
			"failed to generate a status marker: %w",
			err,
		)
	}

	return ssmStatusMarker + hex.EncodeToString(random), nil
}

func shellPath(p string) string {
	switch {
	case p == "~":
		return "~"
	case strings.HasPrefix(p, "~/"):
		return "~/" + shellQuote(strings.TrimPrefix(p, "~/"))
	default:
		return shellQuote(p)
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

type statusFilter struct {
	dest    io.Writer
	marker  string
	pending []byte
	started bool
	code    int
	found   bool
}

func (f *statusFilter) Write(p []byte) (int, error) {
	f.pending = append(f.pending, p...)

	for {
		idx := bytes.IndexByte(f.pending, '\n')
		if idx < 0 {
			return len(p), nil
		}

		line := f.pending[:idx+1]
		f.pending = f.pending[idx+1:]

		if err := f.handleLine(line); err != nil {
			return 0, err
		}
	}
}

func (f *statusFilter) Flush() error {
	if len(f.pending) == 0 {
		return nil
	}

	line := f.pending
	f.pending = nil

	return f.handleLine(line)
}

func (f *statusFilter) Status() (int, bool) {
	return f.code, f.found
}

func (f *statusFilter) handleLine(line []byte) error {
	text := strings.TrimRight(string(line), "\r\n")

	if !f.started {
		if text == "" {
			return nil
		}

		f.started = true

		if strings.HasPrefix(text, ssmPluginBanner) {
			return nil
		}
	}

	if strings.HasPrefix(text, ssmPluginTrailer) {
		return nil
	}

	idx := strings.Index(text, f.marker+"=")
	if idx < 0 {
		return f.forward(line)
	}

	if code, err := strconv.Atoi(
		strings.TrimSpace(text[idx+len(f.marker)+1:]),
	); err == nil {
		f.code = code
		f.found = true
	}

	return f.forward([]byte(text[:idx]))
}

func (f *statusFilter) forward(p []byte) error {
	if len(p) == 0 {
		return nil
	}

	if _, err := f.dest.Write(p); err != nil {
		return errs.Errorf(
			ErrAwsSsmPluginRun,
			"failed to forward remote output: %w",
			err,
		)
	}

	return nil
}

type pluginResponse struct {
	SessionID  string `json:"SessionId"`
	StreamURL  string `json:"StreamUrl"`
	TokenValue string `json:"TokenValue"`
}

func (inv pluginInvocation) pluginArgs() ([]string, []string, error) {
	response, err := json.Marshal(pluginResponse{
		SessionID:  aws.ToString(inv.Output.SessionId),
		StreamURL:  aws.ToString(inv.Output.StreamUrl),
		TokenValue: aws.ToString(inv.Output.TokenValue),
	})
	if err != nil {
		return nil, nil, pluginRunErr(err)
	}

	request, err := json.Marshal(inv.Input)
	if err != nil {
		return nil, nil, pluginRunErr(err)
	}

	args := []string{
		ssmPluginResponseEnv,
		inv.Region,
		ssmPluginOperation,
		inv.Profile,
		string(request),
		"https://ssm." + inv.Region + ".amazonaws.com",
	}

	env := []string{ssmPluginResponseEnv + "=" + string(response)}

	return args, env, nil
}

func pluginRunErr(err error) error {
	return errs.Errorf(
		ErrAwsSsmPluginRun,
		"%s failed: %w",
		ssmPluginBinary,
		err,
	)
}

func runPluginSession(
	ctx context.Context,
	inv pluginInvocation,
	stdout io.Writer,
) error {
	args, env, err := inv.pluginArgs()
	if err != nil {
		return err
	}

	return runPlugin(ctx, ssmPluginBinary, args, env, stdout)
}

func runPlugin(
	ctx context.Context,
	binary string,
	args []string,
	env []string,
	stdout io.Writer,
) error {
	// #nosec G204 -- binary is the plugin's name and argv the session the SDK
	// just started; the token rides in env, out of the process listing.
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = append(cmd.Environ(), env...)
	cmd.Stdout = stdout
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return pluginRunErr(err)
	}

	defer func() { _ = stdin.Close() }()

	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return cancelled(ctxErr)
		}

		return pluginRunErr(err)
	}

	return nil
}

func archivePath(
	localPath string,
	remoteName string,
	info fs.FileInfo,
) ([]byte, error) {
	var buf bytes.Buffer

	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	var err error

	if info.IsDir() {
		err = archiveTree(tw, localPath)
	} else {
		err = archiveEntry(tw, localPath, remoteName, info)
	}

	if err != nil {
		return nil, err
	}

	if err := tw.Close(); err != nil {
		return nil, localPathErr(localPath, err)
	}

	if err := gz.Close(); err != nil {
		return nil, localPathErr(localPath, err)
	}

	return buf.Bytes(), nil
}

func archiveTree(tw *tar.Writer, localDir string) error {
	walk := func(current string, _ fs.DirEntry, err error) error {
		if err != nil {
			return localPathErr(current, err)
		}

		rel, err := filepath.Rel(localDir, current)
		if err != nil {
			return localPathErr(current, err)
		}

		info, err := os.Stat(current)
		if err != nil {
			return localPathErr(current, err)
		}

		return archiveEntry(tw, current, "./"+filepath.ToSlash(rel), info)
	}

	if err := filepath.WalkDir(localDir, walk); err != nil {
		return err
	}

	return nil
}

func archiveEntry(
	tw *tar.Writer,
	localPath string,
	name string,
	info fs.FileInfo,
) error {
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return localPathErr(localPath, err)
	}

	header.Name = name

	if info.IsDir() {
		header.Name = strings.TrimSuffix(name, "/") + "/"
	}

	if err := tw.WriteHeader(header); err != nil {
		return localPathErr(localPath, err)
	}

	if !info.Mode().IsRegular() {
		return nil
	}

	// #nosec G304 -- validated as a project-relative path in config.
	src, err := os.Open(localPath)
	if err != nil {
		return localPathErr(localPath, err)
	}

	defer func() {
		if err := src.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close local file")
		}
	}()

	if _, err := io.Copy(tw, src); err != nil {
		return localPathErr(localPath, err)
	}

	return nil
}

func localPathErr(localPath string, err error) error {
	return errs.Errorf(
		ErrAwsSsmLocalPath,
		"failed to archive %s: %w",
		localPath,
		err,
	)
}
