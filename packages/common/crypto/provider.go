package crypto

type AsymProvider interface {
	GenKeyPair() ([]byte, []byte, error)
	Sign(privateKey, hash []byte) ([]byte, error)
	// Verify checks if forSign has been signed with corresponding to public the private key
	Verify(public, hash, sign []byte) (bool, error)
	PrivateToPublic(key []byte) ([]byte, error)
}

type HashProvider interface {
	// GetHMAC returns HMAC hash
	GetHMAC(secret string, msg string) ([]byte, error)
	// GetHash returns hash of passed bytes
	GetHash(msg []byte) []byte
	// DoubleHash returns double hash of passed bytes
	DoubleHash(msg []byte) []byte
}
