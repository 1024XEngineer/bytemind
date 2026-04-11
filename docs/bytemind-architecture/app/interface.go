package app

import "context"

// Application defines process lifecycle contract.
type Application interface {
	Run(ctx context.Context) error
	Shutdown(ctx context.Context) error
}
