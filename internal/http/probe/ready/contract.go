//go:generate go tool mockgen -source contract.go -destination mocks_test.go -package ready_test
package ready

import "context"

type readinessChecker interface {
	Ping(context.Context) error
}
