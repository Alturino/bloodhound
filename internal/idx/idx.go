package idx

import "context"

type Background interface {
	Start()
	Shutdown()
}

type Seeder interface {
	Seed(ctx context.Context)
}

type Pool[T any] interface {
	Submit(ctx context.Context, tasks []T)
}

type Worker[T any, R any] interface {
	Work(ctx context.Context, task T) (R, error)
}

type WorkerPool[T any, R any] interface {
	Worker[T, R]
	Pool[T]
}
