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

type forwardedBodyKey struct{}

func contextWithForwardedBody(ctx context.Context, b *forwardedBody) context.Context {
	return context.WithValue(ctx, forwardedBodyKey{}, b)
}

func forwardedBodyFromContext(ctx context.Context) (*forwardedBody, bool) {
	b, ok := ctx.Value(forwardedBodyKey{}).(*forwardedBody)
	return b, ok
}
