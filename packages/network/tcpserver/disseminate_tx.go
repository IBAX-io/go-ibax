/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package tcpserver

import (
	"bytes"
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"

	"github.com/IBAX-io/go-ibax/packages/common/crypto/symalgo/aes"

	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/network"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/utils"

	log "github.com/sirupsen/logrus"
)

// DisseminateTxs serves requests from disseminator for tx
func DisseminateTxs(rw io.ReadWriter) error {
	r := &network.DisRequest{}
	if err := r.Read(rw); err != nil {
		return err
	}

	txs, err := UnmarshalTxPacket(r.Data)
	if err != nil {
		return err
	}
	var rtxs []*sqldb.RawTx
	for _, tran := range txs {
		if int64(len(tran)) > syspar.GetMaxTxSize() {
			log.WithFields(log.Fields{"type": consts.ParameterExceeded, "max_tx_size": syspar.GetMaxTxSize(), "current_size": len(tran)}).Error("transaction size exceeds max size")
			return utils.ErrInfo("len(txBinData) > max_tx_size")
		}

		if tran == nil {
			log.WithFields(log.Fields{"type": consts.ParameterExceeded, "mx_tx_size": syspar.GetMaxTxSize(), "info": "tran nil", "current_size": len(tran)}).Error("transaction size nil")
			continue
		}

		rtx := &transaction.Transaction{}
		if err = rtx.Unmarshall(bytes.NewBuffer(tran), true); err != nil {
			return err
		}
		rtxs = append(rtxs, rtx.SetRawTx())
	}

	err = sqldb.SendTxBatches(rtxs)
	if err != nil {
		return err
	}
	return nil
}

//// Type2 serves requests from disseminator
//func Type2(rw io.ReadWriter) (*network.DisTrResponse, error) {
//	r := &network.DisRequest{}
//	if err := r.Read(rw); err != nil {
//		return nil, err
//	}
//
//	binaryData := r.Data
//	// take the transactions from usual users but not nodes.
//	_, _, decryptedBinData, err := DecryptData(&binaryData)
//	if err != nil {
//		return nil, utils.ErrInfo(err)
//	}
//
//	if int64(len(binaryData)) > syspar.GetMaxTxSize() {
//		log.WithFields(log.Fields{"type": consts.ParameterExceeded, "max_size": syspar.GetMaxTxSize(), "size": len(binaryData)}).Error("transaction size exceeds max size")
//		return nil, utils.ErrInfo("len(txBinData) > max_tx_size")
//	}
//
//	if len(binaryData) < 5 {
//		log.WithFields(log.Fields{"type": consts.ProtocolError, "len": len(binaryData), "should_be_equal": 5}).Error("binary data slice has incorrect length")
//		return nil, utils.ErrInfo("len(binaryData) < 5")
//	}
//
//	tx := transaction.Transaction{}
//	if err = tx.Unmarshall(bytes.NewBuffer(decryptedBinData)); err != nil {
//		log.WithFields(log.Fields{"type": consts.UnmarshallingError, "error": err}).Error("unmarshalling transaction")
//		return nil, err
//	}
//
//	_, err = sqldb.DeleteQueueTxByHash(nil, tx.Hash())
//	if err != nil {
//		log.WithFields(log.Fields{"type": consts.DBError, "error": err, "hash": tx.Hash()}).Error("Deleting queue_tx with hash")
//		return nil, utils.ErrInfo(err)
//	}
//
//	queueTx := &sqldb.QueueTx{Hash: tx.Hash(), Data: decryptedBinData, FromGate: 0}
//	err = queueTx.Create()
//	if err != nil {
//		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("Creating queue_tx")
//		return nil, utils.ErrInfo(err)
//	}
//
//	return &network.DisTrResponse{}, nil
//}

// DecryptData is decrypting data
func DecryptData(binaryTx *[]byte) ([]byte, []byte, []byte, error) {
	if len(*binaryTx) == 0 {
		log.WithFields(log.Fields{"type": consts.EmptyObject}).Error("binary tx is empty")
		return nil, nil, nil, utils.ErrInfo("len(binaryTx) == 0")
	}

	myUserID := converter.BinToDecBytesShift(&*binaryTx, 5)
	log.WithFields(log.Fields{"user_id": myUserID}).Debug("decrypted userID is")

	// remove the encrypted key, and all that stay in $binary_tx will be encrypted keys of the transactions/blocks
	length, err := converter.DecodeLength(&*binaryTx)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.ProtocolError, "error": err}).Error("Decoding binary tx length")
		return nil, nil, nil, err
	}
	encryptedKey := converter.BytesShift(&*binaryTx, length)
	iv := converter.BytesShift(&*binaryTx, 16)
	log.WithFields(log.Fields{"encryptedKey": encryptedKey, "iv": iv}).Debug("binary tx encryptedKey and iv is")

	if len(encryptedKey) == 0 {
		log.WithFields(log.Fields{"type": consts.EmptyObject}).Error("binary tx encrypted key is empty")
		return nil, nil, nil, utils.ErrInfo("len(encryptedKey) == 0")
	}

	if len(*binaryTx) == 0 {
		log.WithFields(log.Fields{"type": consts.EmptyObject}).Error("binary tx is empty")
		return nil, nil, nil, utils.ErrInfo("len(*binaryTx) == 0")
	}

	nodeKeyPrivate, _ := utils.GetNodeKeys()
	if len(nodeKeyPrivate) == 0 {
		log.WithFields(log.Fields{"type": consts.EmptyObject}).Error("node private key is empty")
		return nil, nil, nil, utils.ErrInfo("len(nodePrivateKey) == 0")
	}

	block, _ := pem.Decode([]byte(nodeKeyPrivate))
	if block == nil || block.Type != "RSA PRIVATE KEY" {
		log.WithFields(log.Fields{"type": consts.CryptoError}).Error("No valid PEM data found")
		return nil, nil, nil, utils.ErrInfo("No valid PEM data found")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.CryptoError, "error": err}).Error("Parse PKCS1PrivateKey")
		return nil, nil, nil, utils.ErrInfo(err)
	}

	decKey, err := rsa.DecryptPKCS1v15(crand.Reader, privateKey, encryptedKey)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.CryptoError, "error": err}).Error("rsa Decrypt")
		return nil, nil, nil, utils.ErrInfo(err)
	}

	log.WithFields(log.Fields{"key": decKey}).Debug("decrypted key")
	if len(decKey) == 0 {
		log.WithFields(log.Fields{"type": consts.EmptyObject}).Error("decrypted key is empty")
		return nil, nil, nil, utils.ErrInfo("len(decKey)")
	}

	log.WithFields(log.Fields{"binaryTx": *binaryTx, "iv": iv}).Debug("binaryTx and iv is")
	decrypted, err := aes.Decrypt(iv, *binaryTx, decKey)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.CryptoError, "error": err}).Error("Decryption binary tx")
		return nil, nil, nil, utils.ErrInfo(err)
	}

	return decKey, iv, decrypted, nil
}

func UnmarshalTxPacket(dat []byte) ([][]byte, error) {
	var txes [][]byte
	err := json.Unmarshal(dat, &txes)
	return txes, err
}
