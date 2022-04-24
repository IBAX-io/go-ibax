/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package ecies

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"runtime"

	"github.com/IBAX-io/go-ibax/packages/consts"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
)

func init() {
	log.SetFlags(log.Ldate | log.Lshortfile)
}

//
//Ecc
/*func EccEnCrypt(plainText []byte,prv2 *ecies.PrivateKey)(crypText []byte,err error){

	ct, err := ecies.Encrypt(rand.Reader, &prv2.PublicKey, plainText, nil, nil)
	return ct, err
}
//
func EccDeCrypt(cryptText []byte,prv2 *ecies.PrivateKey) ([]byte, error) {
	pt, err := prv2.Decrypt(cryptText, nil, nil)
	return pt, err
}*/

//
func EccPubEncrypt(plainText []byte, pub *ecdsa.PublicKey) (cryptText []byte, err error) { //

	defer func() {
		if err := recover(); err != nil {
			switch err.(type) {
			case runtime.Error:
				log.Println("runtime err:", err, "check key ")
			default:
				log.Println("error:", err)
			}
		}
	}()

	publicKey := ImportECDSAPublic(pub)
	//
	crypttext, err := Encrypt(rand.Reader, publicKey, plainText, nil, nil)

	return crypttext, err

}

//
func EccPriDeCrypt(cryptText []byte, priv *ecdsa.PrivateKey) (msg []byte, err error) { //
	privateKey := ImportECDSA(priv)

	//
	plainText, err := privateKey.Decrypt(cryptText, nil, nil)

	return plainText, err
}

func EccCryptoKey(plainText []byte, publickey string) (cryptoText []byte, err error) {
	pubbuff, err := crypto.HexToPub(publickey)
	if err != nil {
		return nil, err
	}
	pub, err := GetPublicKeys(pubbuff)
	if err != nil {
		return nil, err
	}
	return EccPubEncrypt(plainText, pub)
}

func EccDeCrypto(cryptoText []byte, prikey []byte) ([]byte, error) {
	pri, err := GetPrivateKeys(prikey)
	if err != nil {
		return nil, err
	}
	return EccPriDeCrypt(cryptoText, pri)
}

// GetPrivateKeys return
func GetPrivateKeys(privateKey []byte) (ret *ecdsa.PrivateKey, err error) {
	var pubkeyCurve elliptic.Curve
	pubkeyCurve = elliptic.P256()
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

	pubkey.Curve = elliptic.P256()
	pubkey.X = new(big.Int).SetBytes(public[0:consts.PrivkeyLength])
	pubkey.Y = new(big.Int).SetBytes(public[consts.PrivkeyLength:])
	return pubkey, nil
}
