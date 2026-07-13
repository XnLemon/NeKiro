package catalog

import (
	"context"
	"time"
)

// Store owns durable Registry facts and the transactionally derived discovery index.
type Store interface {
	Register(context.Context, AgentVersion) (AgentVersion, error)
	Get(context.Context, string, string) (AgentVersion, error)
	Publish(context.Context, string, string, string, time.Time) (AgentVersion, error)
	Disable(context.Context, string, string, string, time.Time) (AgentVersion, error)
	DiscoverFirstPage(context.Context, DiscoveryFilter) (int64, DiscoveryResult, error)
	Discover(context.Context, DiscoveryQuery) (DiscoveryResult, error)
	Check(context.Context) error
}
