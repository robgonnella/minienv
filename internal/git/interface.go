package git

type Client interface {
	ShortSha() (string, error)
}
