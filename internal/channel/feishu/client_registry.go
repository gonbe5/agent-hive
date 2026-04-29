package feishu

import "fmt"

// ClientRegistry provides tenant-scoped client lookup.
// Phase 0 uses a single client, but all future tenant fan-out must enter here.
type ClientRegistry interface {
	Get(tenantKey string) (*Client, error)
	Register(tenantKey string, client *Client) error
	Unregister(tenantKey string) error
	List() []string
}

type SingleClientRegistry struct {
	client *Client
	key    string
}

func NewSingleClientRegistry(client *Client, tenantKey string) *SingleClientRegistry {
	if tenantKey == "" {
		tenantKey = DefaultTenantKey
	}
	return &SingleClientRegistry{
		client: client,
		key:    tenantKey,
	}
}

func (r *SingleClientRegistry) Get(tenantKey string) (*Client, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("feishu client registry is empty")
	}
	if tenantKey == "" {
		tenantKey = DefaultTenantKey
	}
	if tenantKey != r.key && tenantKey != DefaultTenantKey {
		return nil, fmt.Errorf("tenant not found: %s", tenantKey)
	}
	return r.client, nil
}

func (r *SingleClientRegistry) Register(tenantKey string, client *Client) error {
	if r == nil {
		return fmt.Errorf("feishu client registry is nil")
	}
	if tenantKey == "" {
		tenantKey = DefaultTenantKey
	}
	if tenantKey != r.key {
		return fmt.Errorf("single client registry only supports tenant: %s", r.key)
	}
	if client == nil {
		return fmt.Errorf("feishu client is nil")
	}
	r.client = client
	return nil
}

func (r *SingleClientRegistry) Unregister(tenantKey string) error {
	if r == nil {
		return fmt.Errorf("feishu client registry is nil")
	}
	if tenantKey == "" {
		tenantKey = DefaultTenantKey
	}
	if tenantKey != r.key {
		return fmt.Errorf("tenant not found: %s", tenantKey)
	}
	r.client = nil
	return nil
}

func (r *SingleClientRegistry) List() []string {
	if r == nil || r.client == nil || r.key == "" {
		return nil
	}
	return []string{r.key}
}
