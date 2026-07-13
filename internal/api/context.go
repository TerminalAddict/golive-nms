package api

import (
	"context"
	"net/http"
	"time"

	"github.com/golive-nms/golive-nms/internal/store"
)

func contextTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}

type userKey struct{}

func WithUser(ctx context.Context, u store.User) context.Context {
	return context.WithValue(ctx, userKey{}, u)
}
func CurrentUser(ctx context.Context) (store.User, bool) {
	u, ok := ctx.Value(userKey{}).(store.User)
	return u, ok
}
