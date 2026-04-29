package idx

import "context"

type Pool[T any] interface {
	Submit(ctx context.Context, task *T)
	Shutdown()
}
