	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
