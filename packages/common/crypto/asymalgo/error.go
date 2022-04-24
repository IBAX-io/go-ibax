package asymalgo

import "errors"

var (
	// ErrSigningEmpty is Signing empty value error
	ErrSigningEmpty = errors.New("Signing empty value")
	// ErrCheckingSignEmpty is Checking sign of empty error
	ErrCheckingSignEmpty = errors.New("Cheking sign of empty")
	// ErrIncorrectSign is Incorrect sign
	ErrIncorrectSign = errors.New("Incorrect sign")
)
