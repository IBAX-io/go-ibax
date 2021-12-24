package crypto

import (
	"fmt"
	"testing"
)

func TestGetHasher(t *testing.T) {
	h := getHasher()
	msg := []byte("Hello")
	fmt.Println(h.hash(msg))

	message := "Hello"
	secret := "world"

	hmacMsg, err := h.getHMAC(secret, message)

	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(hmacMsg)
}
