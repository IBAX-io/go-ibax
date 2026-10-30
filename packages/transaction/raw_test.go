/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package transaction

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/types"
)

// A frame of a client is read only where a client sends it: a block, another node and the pool
// carry the stored form, which the node a client sends to makes with its own time. Read in a
// block, every node would store the frame with a time of its own, and they would disagree.
func TestUnmarshallClientFrame(t *testing.T) {
	var every syspar.AccountAlgorithmSet
	for _, a := range []crypto.AsymAlgo{crypto.AsymAlgo_ECC_P256, crypto.AsymAlgo_ECC_Secp256k1, crypto.AsymAlgo_SM2, crypto.AsymAlgo_MLDSA65, crypto.AsymAlgo_MLDSA87} {
		every = append(every, syspar.AccountAlgorithm{Algo: a})
	}
	syspar.SetAccountAlgorithms(every)
	defer syspar.SetAccountAlgorithms(nil)
	for _, c := range loadTransfers(t) {
		crypto.InitAsymAlgo(c.Cryptoer)
		crypto.InitHashAlgo(c.Hasher)
		conf.Config.LocalConf.NetworkID = c.NetworkID
		data, err := hex.DecodeString(c.Data)
		if err != nil {
			t.Fatal(err)
		}
		name := c.Cryptoer + "/" + c.Hasher
		if data[0] != types.ClientTxType {
			t.Fatalf("%s: the vector is no frame of a client: type %d", name, data[0])
		}
		if err := (&Transaction{}).Unmarshall(bytes.NewBuffer(bytes.Clone(data)), true); err == nil {
			t.Errorf("%s: a frame of a client read as a stored transaction", name)
		}
		if _, err := UnmarshallTransaction(bytes.NewBuffer(bytes.Clone(data)), false); err == nil {
			t.Errorf("%s: a frame of a client read as a transaction of a block", name)
		}
		client := &Transaction{}
		if err := client.UnmarshallClient(bytes.NewBuffer(bytes.Clone(data)), true); err != nil {
			t.Fatalf("%s: the frame refused where a client sends it: %v", name, err)
		}
		if client.FullData[0] == types.ClientTxType {
			t.Fatalf("%s: the frame of the client was not stored", name)
		}
		for _, read := range []func(*Transaction, *bytes.Buffer, bool) error{(*Transaction).Unmarshall, (*Transaction).UnmarshallClient} {
			stored := &Transaction{}
			if err := read(stored, bytes.NewBuffer(bytes.Clone(client.FullData)), true); err != nil {
				t.Fatalf("%s: the stored form refused: %v", name, err)
			}
			if !bytes.Equal(stored.FullData, client.FullData) || !bytes.Equal(stored.Hash(), client.Hash()) {
				t.Errorf("%s: the stored form read back differs", name)
			}
		}
	}
}
