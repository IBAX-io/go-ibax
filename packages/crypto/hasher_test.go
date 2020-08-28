package crypto

import (
	"fmt"
	"testing"

	hmacMsg, err := h.getHMAC(secret, message)

	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(hmacMsg)
}
