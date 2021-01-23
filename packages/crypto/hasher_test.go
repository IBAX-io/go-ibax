package crypto
	fmt.Println(h.hash(msg))

	message := "Hello"
	secret := "world"

	hmacMsg, err := h.getHMAC(secret, message)

	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(hmacMsg)
}
