package testutils

import (
	"strings"
)

// ContainsError While testing there is no point in mimicking wrapped error hierarchies, but we do want to distinguish check
// whether an error is part of the stacktrace
// when error message itself contains :, the split contains logic is not enough, add compare of error message itself
func ContainsError(wrappedErr error, containedErr error) bool {
	if containedErr == nil && wrappedErr == nil {
		return true
	}
	if containedErr == nil || wrappedErr == nil {
		return false
	}

	errMsg := wrappedErr.Error()
	split := strings.Split(errMsg, ":")
	for _, v := range split {
		if strings.TrimSpace(v) == containedErr.Error() {
			return true
		}
	}
	return errMsg == containedErr.Error() 
}
