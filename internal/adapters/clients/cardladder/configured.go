package cardladder

import (
	"context"
	"sync"

	domain "github.com/guarzo/slabledger/internal/domain/cardladder"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/showprep"
)

type ConfigReader interface {
	GetConfig(context.Context) (*domain.Config, error)
}

// ConfiguredClient owns one process-wide client/limiter, including late setup.
// It compares saved credentials, not the client's rotating token. Refresh is a
// database-only sync; only Source hands acquisition capability to the worker.
type ConfiguredClient struct {
	mu                   sync.Mutex
	refreshing           chan struct{}
	store                ConfigReader
	client               *Client
	savedKey, savedToken string
	configured           bool
	logger               observability.Logger
	authOptions          []AuthOption
	consumer             func(*Client)
}

func NewConfiguredClient(store ConfigReader, client *Client, logger observability.Logger, authOptions ...AuthOption) *ConfiguredClient {
	return &ConfiguredClient{store: store, client: client, configured: client != nil && client.Available(), logger: logger, authOptions: authOptions, refreshing: make(chan struct{}, 1)}
}

// SetConsumer connects the existing CL refresher's SetClient activation path.
func (c *ConfiguredClient) SetConsumer(consumer func(*Client)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.consumer = consumer
	if consumer != nil {
		consumer(c.current())
	}
}
func (c *ConfiguredClient) current() *Client {
	if !c.configured {
		return nil
	}
	return c.client
}
func (c *ConfiguredClient) Current() *Client { c.mu.Lock(); defer c.mu.Unlock(); return c.current() }

func (c *ConfiguredClient) Configured(ctx context.Context) (bool, error) {
	if c.store == nil {
		return false, nil
	}
	cfg, err := c.store.GetConfig(ctx)
	return cfg != nil && cfg.FirebaseAPIKey != "" && cfg.RefreshToken != "", err
}

func (c *ConfiguredClient) Refresh(ctx context.Context) error {
	// Serialize reads + activation without making shutdown wait on another
	// caller's database context. Current/Configured remain independent reads.
	select {
	case c.refreshing <- struct{}{}:
		defer func() { <-c.refreshing }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var cfg *domain.Config
	if c.store != nil {
		var err error
		cfg, err = c.store.GetConfig(ctx)
		if err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	configured := cfg != nil && cfg.FirebaseAPIKey != "" && cfg.RefreshToken != ""
	if !configured {
		if c.configured && c.client != nil {
			c.client.UpdateCredentials(nil, "")
		}
		c.configured = false
		c.savedKey = ""
		c.savedToken = ""
	} else {
		if c.client == nil {
			c.client = NewClient()
		}
		if !c.configured || cfg.FirebaseAPIKey != c.savedKey || cfg.RefreshToken != c.savedToken {
			c.client.UpdateCredentials(NewFirebaseAuth(cfg.FirebaseAPIKey, c.authOptions...), cfg.RefreshToken)
		}
		c.configured = true
		c.savedKey = cfg.FirebaseAPIKey
		c.savedToken = cfg.RefreshToken
	}
	if c.consumer != nil {
		c.consumer(c.current())
	}
	return nil
}

func (c *ConfiguredClient) Authenticate(ctx context.Context, apiKey, email, password string) (*FirebaseAuthResponse, error) {
	return NewFirebaseAuth(apiKey, c.authOptions...).Login(ctx, email, password)
}

func (c *ConfiguredClient) Source(ctx context.Context) (showprep.Source, error) {
	if err := c.Refresh(ctx); err != nil {
		return nil, err
	}
	client := c.Current()
	if client == nil {
		return nil, nil
	}
	// Do not reuse a wrapper across credential generations: it owns singleflight.
	return NewShowPrepSource(client, c.logger), nil
}
