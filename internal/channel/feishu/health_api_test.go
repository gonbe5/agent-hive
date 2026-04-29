package feishu

import "errors"

func newPermissionDeniedError(message string) error {
	return errors.New(message)
}
