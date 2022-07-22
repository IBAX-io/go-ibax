package smart

import (
	"fmt"
	"testing"
)

func TestAddressToID(t *testing.T) {
	for i := 0; i <= 9999; i++ {
		si := fmt.Sprintf("%04d", i)
		for j := 0; j <= 9999; j++ {
			sj := fmt.Sprintf("%04d", j)
			id := AddressToID("2222-2222-2222-" + si + "-" + sj)
			if id != 0 {
				fmt.Println(id)
			}
		}
	}

	ss := fmt.Sprintf("%04d", 1)
	fmt.Println(ss)

}

func TestAddressToID_0(t *testing.T) {
	for i := 0; i <= 9999; i++ {
		si := fmt.Sprintf("%04d", i)
		address := "0000-0000-0000-0000-" + si
		id := AddressToID(address)
		if id != 0 {
			address2 := IDToAddress(id)
			fmt.Println(i, id, address, address == address2)
		}
	}
}

func TestAddressToID_1(t *testing.T) {
	for i := 0; i <= 9999; i++ {
		si := fmt.Sprintf("%04d", i)
		address := "1111-1111-1111-1111-" + si
		id := AddressToID(address)
		if id != 0 {
			address2 := IDToAddress(id)
			fmt.Println(i, id, address, address == address2)
		}
	}
}
