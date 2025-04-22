package internal

import (
	"github.com/pkg/errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// VerifyTestError verifies whether a given error is contained in the wrapped chain of errors if an error is expected,
// otherwise verifies that no error is thrown / passed
func VerifyTestError(t *testing.T, expectedErr error, gotErr error) {
	if gotErr != nil {
		assert.Truef(t, errors.Is(gotErr, expectedErr), "expected error %v, got %v", expectedErr, gotErr)
		return
	}
	if expectedErr != nil {
		t.Errorf("expected error %v, got nil", expectedErr.Error())
	}
}
