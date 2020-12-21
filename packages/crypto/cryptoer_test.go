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
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(ok)
}
