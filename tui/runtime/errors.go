package runtime

import "errors"

func IsUnavailableError(err error) bool {
	return errors.Is(err, errRunnerUnavailable) || errors.Is(err, errStoreUnavailable)
}
