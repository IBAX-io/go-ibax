/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/IBAX-io/go-ibax/packages/consts"
)

func SignString(privateKeyHex, data string) ([]byte, error) {
	privateKey, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("decoding private key from hex: %w", err)
	}
	return getCryptoer().sign(privateKey, []byte(data))
}

// GetPrivateKeys return
func GetPrivateKeys(privateKey []byte) (ret *ecdsa.PrivateKey, err error) {
	var pubkeyCurve elliptic.Curve

	switch ellipticSize {
	case elliptic256:
		pubkeyCurve = elliptic.P256()
	default:
		return nil, ErrUnsupportedCurveSize
	}

	bi := new(big.Int).SetBytes(privateKey)
	priv := new(ecdsa.PrivateKey)
	priv.PublicKey.Curve = pubkeyCurve
	priv.D = bi

	return priv, nil
}

// GetPublicKeys return
func GetPublicKeys(public []byte) (*ecdsa.PublicKey, error) {

	pubkey := new(ecdsa.PublicKey)

	if len(public) != consts.PubkeySizeLength {
		return pubkey, fmt.Errorf("invalid parameters len(public) = %d", len(public))
	}

	var pubkeyCurve elliptic.Curve
	switch ellipticSize {
	case elliptic256:
		pubkeyCurve = elliptic.P256()
	default:
		return nil, ErrUnsupportedCurveSize
	}

	pubkey.Curve = pubkeyCurve
	pubkey.X = new(big.Int).SetBytes(public[0:consts.PrivkeyLength])
	pubkey.Y = new(big.Int).SetBytes(public[consts.PrivkeyLength:])

	return pubkey, nil
}
