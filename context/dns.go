package context

import (
	"context"
)

type DNSContext struct {
	context.Context
}

func NewDNSContext(ctx context.Context) *DNSContext {
	return &DNSContext{
		Context: ctx,
	}
}
