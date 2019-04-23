package crypto

import (
	"encoding/hex"
	"fmt"
	"log"
	"testing"
)

func TestGetCryptoer(t *testing.T) {
	c := getCryptoer()
	src := []byte("Hello")
	encodedStr := hex.EncodeToString(src)
	fmt.Println(src)
	fmt.Printf("%s\n", encodedStr)
	prv, pub, err := c.genKeyPair()
	if err != nil {
		return
	}
	prvStr := hex.EncodeToString(prv)
	pubStr := hex.EncodeToString(pub)
	fmt.Println("privateKey is:", prv, "publicKey", pub)
	fmt.Println("privateKeyString is:", prvStr, "publicKeyString is:", pubStr)
	addr := Address(pub)

	fmt.Println("Address is:", addr)
	signedDataByte, err := c.sign(prv, src)
	if err != nil {
		log.Fatal(err)
	}
	signedDataStr := hex.EncodeToString(signedDataByte)
	fmt.Println("signedDataByte is:", signedDataByte)
	fmt.Println("signedDataString is:", signedDataStr)

	// priv test
	// signedDataString := "3045022100929be5f360d10270bc67b6f9d28c47c257e472fdbdf66a3037022e47143bf94c0220246f9e378444d1d0fa81f613fb93c3c420e0a1abd0f3138cf10788492f690fc8"
	// signedDataByte, err := hex.DecodeString(signedDataString)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// 	log.Fatal(err)
	// }
	fmt.Println("signedDataByPriv is:", signedDataByte)
	ok, err := c.verify(pub, src, signedDataByte)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(ok)
}
