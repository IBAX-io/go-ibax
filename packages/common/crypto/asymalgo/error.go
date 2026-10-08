package asymalgo

import (
	"errors"
	"math/big"
)

var (
	// ErrSigningEmpty is Signing empty value error
	ErrSigningEmpty = errors.New("Signing empty value")
	// ErrCheckingSignEmpty is Checking sign of empty error
	ErrCheckingSignEmpty = errors.New("Cheking sign of empty")
	// ErrIncorrectSign is Incorrect sign
	ErrIncorrectSign = errors.New("Incorrect sign")
	// ErrInvalidPrivateKey is a private key outside [1, n-1] of the curve
	ErrInvalidPrivateKey = errors.New("Invalid private key")
)

// privateScalar reads a private key as the curve scalar it must be: 1 <= d < n. A zero key signs
// without error on some curves, so an unloaded key would otherwise go unnoticed.
func privateScalar(privateKey []byte, n *big.Int) (*big.Int, error) {
	d := new(big.Int).SetBytes(privateKey)
	if d.Sign() <= 0 || d.Cmp(n) >= 0 {
		return nil, ErrInvalidPrivateKey
	}
	return d, nil
}
