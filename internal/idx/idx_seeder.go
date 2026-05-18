package idx

import "context"

type Seeder interface {
	Seed(ctx context.Context)
}
