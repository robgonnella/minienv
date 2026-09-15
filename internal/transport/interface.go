package transport

// A Client may hold an open connection, so callers must Close it. Close is a
// no-op if nothing was dialed. Not safe for concurrent use.
type Client interface {
	String() string
	CreateFile(filepath string, content []byte) error
	CopyPath(localPath, remotePath string) error
	RunCommand(cmd string) error
	Close() error
}
