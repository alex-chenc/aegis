package client

// Client stub - gRPC client to be implemented with complete protobuf definitions
type Client struct{}

func NewClient(grpcAddr, hostID string, maxConcurrency int) *Client {
	return &Client{}
}

func (c *Client) Connect() error { return nil }
func (c *Client) Close() {}

