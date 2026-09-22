package transport

import "context"

// A Client may hold an open connection, so callers must Close it. Close is a
// no-op if nothing was dialed. Not safe for concurrent use.
type Client interface {
	String() string
	CreateFile(ctx context.Context, filepath string, content []byte) error
	CopyPath(ctx context.Context, localPath, remotePath string) error
	RunCommand(ctx context.Context, cmd string) error
	Close() error
}
