/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package ecies

import (
	"crypto/ecdsa"
	"crypto/rand"
	"log"
	"runtime"

	"github.com/IBAX-io/go-ibax/packages/crypto"
)

func init() {
	log.SetFlags(log.Ldate | log.Lshortfile)
}

//
//Ecc
/*func EccEnCrypt(plainText []byte,prv2 *ecies.PrivateKey)(crypText []byte,err error){

	ct, err := ecies.Encrypt(rand.Reader, &prv2.PublicKey, plainText, nil, nil)
	return ct, err
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
	pub, err := crypto.GetPublicKeys(pubbuff)
	if err != nil {
		return nil, err
	}
	return EccPubEncrypt(plainText, pub)
}

func EccDeCrypto(cryptoText []byte, prikey []byte) ([]byte, error) {
	pri, err := crypto.GetPrivateKeys(prikey)
	if err != nil {
		return nil, err
	}
	return EccPriDeCrypt(cryptoText, pri)
}
