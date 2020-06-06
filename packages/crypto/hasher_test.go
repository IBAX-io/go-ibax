package crypto

import (
	"fmt"
	"testing"
)

func TestGetHasher(t *testing.T) {
	h := getHasher()
	msg := []byte("Hello")
	hmacMsg, err := h.getHMAC(secret, message)

	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(hmacMsg)
}
