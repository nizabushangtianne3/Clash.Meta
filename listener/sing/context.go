package sing

import (
	"context"
	"net"
	"slices"

	"github.com/metacubex/mihomo/adapter/inbound"

	"github.com/metacubex/sing/common/auth"
)

// A key travels through context.Context as an any, so a value-shaped key
// boxes into the interface on every lookup, while a pointer is held as is.
// Each key is its own pointer, which keeps them distinct.
type contextKey struct{}

var ctxKeyAdditions = &contextKey{}

func WithAdditions(ctx context.Context, additions ...inbound.Addition) context.Context {
	return context.WithValue(ctx, ctxKeyAdditions, additions)
}

func AdditionsFromContext(ctx context.Context) []inbound.Addition { return getAdditions(ctx) }

func getAdditions(ctx context.Context) (additions []inbound.Addition) {
	if v := ctx.Value(ctxKeyAdditions); v != nil {
		if a, ok := v.([]inbound.Addition); ok {
			additions = a
		}
	}
	if user, ok := auth.UserFromContext[string](ctx); ok {
		additions = slices.Clip(additions) // force the subsequent `append()` to copy the slice
		additions = append(additions, inbound.WithInUser(user))
	}
	return
}

var ctxKeyInAddr = &contextKey{}

func WithInAddr(ctx context.Context, inAddr net.Addr) context.Context {
	return context.WithValue(ctx, ctxKeyInAddr, inAddr)
}

func getInAddr(ctx context.Context) net.Addr {
	if v := ctx.Value(ctxKeyInAddr); v != nil {
		if a, ok := v.(net.Addr); ok {
			return a
		}
	}
	return nil
}
