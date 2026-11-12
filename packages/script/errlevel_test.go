package script

import (
	"errors"
	"fmt"
	"testing"

	log "github.com/sirupsen/logrus"
)

type rerunTestError struct{}

func (rerunTestError) Error() string { return "runs again" }

func (rerunTestError) Rerun() bool { return true }

// TestErrLevel checks the error after which a transaction runs again, and the time of the block
// that stops it, stay under the places of the contracts that met them, and log at no error level
func TestErrLevel(t *testing.T) {
	placed := &placedError{text: "runs again [DBInsert @1A:3] @1B:5]", cause: fmt.Errorf("%w [DBInsert @1A:3]", rerunTestError{})}
	if l := errLevel(placed); l != log.DebugLevel {
		t.Fatalf("level %v of a rerun", l)
	}
	timed := &placedError{text: "time limit exceeded [@1A:3] @1B:5]", cause: fmt.Errorf("%w [@1A:3]", ErrVMTimeLimit)}
	if l := errLevel(timed); l != log.DebugLevel {
		t.Fatalf("level %v of the time of the block", l)
	}
	if l := errLevel(errors.New("runs again")); l != log.ErrorLevel {
		t.Fatalf("level %v of a fault", l)
	}
}
