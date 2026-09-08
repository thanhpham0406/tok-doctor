package gateway

import "context"

type exchangeKey struct{}

func contextWithExchange(ctx context.Context, e *Exchange) context.Context {
	return context.WithValue(ctx, exchangeKey{}, e)
}

func exchangeFromContext(ctx context.Context) (*Exchange, bool) {
	e, ok := ctx.Value(exchangeKey{}).(*Exchange)
	return e, ok
}
