package api

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/common/crypto/asymalgo"
	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
	cryptoeth "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	"math/big"
	"testing"
)

func init() {
	crypto.InitAsymAlgo("ECC_Secp256k1")
	crypto.InitHashAlgo("KECCAK256")
}

//ECC_Secp256k1
//SHA3_256
//sign fe0d9462c700c3c27160a01fad959b9a4b01dd15742dec20343fa0c8af224b302d4f615b83145f19266f0e416639dcd5c5997b7aa21629997b8508a50ceb9309
//pub 043b7808c58fd8a5dd224dc051211eadb4f66900042695deb2f7e30b4825e610a6c641461e52b4121e27af0deea6d3ad3b71636c5967d51cb06acf930851efe34a
//KECCAK256
//sign 205d11f2a5f1163dc8578fb1cad2e16d292db0111065a6bf0ee3f01e602a3b382206bfacd8543ceec8463279275fa747c09b6e3c99945ea50162b3902d89b418
//hash 49ea7d9596821209e27d87ea1b1291e11edcd875f7eb5ad2baa44a6378af38e1

func TestSign(t *testing.T) {
	key := "a1f327a0cf99fb47bd42dea05ed948377dc3d0804154ce2ea4f05ec1b35dabeb"
	prv, _ := hex.DecodeString(key)
	pub1, _ := PrivateToPublicHex(key)
	pub2, _ := hex.DecodeString(pub1)
	pub := pub2[1:]
	fmt.Println("prv1", hex.EncodeToString(prv))
	//fmt.Println("pub1", pub1)
	//fmt.Println("pub2", hex.EncodeToString(pub))
	data := []byte("LOGIN")
	//prv, pub, _ := crypto.GenKeyPair()
	sign, _ := crypto.Sign(prv, data)
	hash := crypto.Hash(data)
	fmt.Println("hash", hex.EncodeToString(hash))
	fmt.Println("sign", hex.EncodeToString(sign))
	fmt.Println("pub", hex.EncodeToString(pub))

	sign, _ = hex.DecodeString("06414d30cdede937d7ff9ae4539c0953ce33a8b5abd399592b9e0816ce5877a2883be349237eb6647a78cea52896a1b6ce8357d6aa3ce56ee461b828b0bcdc02")

	verify, _ := crypto.Verify(pub, data, sign)
	fmt.Println("verify", verify)
 
}

func TestSecp256k1(t *testing.T) {
	key := "a1f327a0cf99fb47bd42dea05ed948377dc3d0804154ce2ea4f05ec1b35dabeb"
	prv, _ := hex.DecodeString(key)
	pub1, _ := PrivateToPublicHex(key)
	pub2, _ := hex.DecodeString(pub1)
	pub := pub2[1:]
	//prv, pub, _ := crypto.GenKeyPair()
	data := []byte("LOGIN")
	hash := crypto.Hash(data)
	signature, err := secp256k1.Sign(hash, prv)
	if err != nil {
		panic(err)
	}
	fmt.Println("sign", hex.EncodeToString(signature))
	ok := secp256k1.VerifySignature(pub, hash, signature)
	fmt.Println("ok", ok)

}

func TestSecp256k1ETH(t *testing.T) {
	prvHex := "a1f327a0cf99fb47bd42dea05ed948377dc3d0804154ce2ea4f05ec1b35dabeb"
	key, _ := hex.DecodeString(prvHex)
	pubkeyCurve := secp.S256()
	priv := new(ecdsa.PrivateKey)
	priv.PublicKey.Curve = pubkeyCurve
	priv.D = new(big.Int).SetBytes(key)
	priv.PublicKey.X, priv.PublicKey.Y = pubkeyCurve.ScalarBaseMult(key)
	prv := priv.D.Bytes()
	pub := append(asymalgo.FillLeft(priv.PublicKey.X.Bytes()), asymalgo.FillLeft(priv.PublicKey.Y.Bytes())...)

	data := []byte("LOGIN")
	hash := crypto.Hash(data)
	fmt.Println("hash", hex.EncodeToString(hash))
	signature, err := secp256k1.Sign(hash, prv)
	if err != nil {
		panic(err)
	}
	fmt.Println("sign", hex.EncodeToString(signature))
	ok := secp256k1.VerifySignature(pub, hash, signature)
	fmt.Println("ok", ok)
	hash2 := cryptoeth.Keccak256(data)
	fmt.Println("hash2", hex.EncodeToString(hash2))
	sig, _ := cryptoeth.Sign(hash2, priv)
	fmt.Println("sign2", hex.EncodeToString(sig))
	recPubkey, err := cryptoeth.SigToPub(hash2, sig)
	if err != nil {
		panic(err)
	}
	fmt.Println("pub ", hex.EncodeToString(pub))
	fmt.Println("pub2", hex.EncodeToString(recPubkey.X.Bytes()), hex.EncodeToString(recPubkey.Y.Bytes()))
}
