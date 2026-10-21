/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/script"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/transaction"
	"github.com/IBAX-io/go-ibax/packages/types"

	log "github.com/sirupsen/logrus"
)

// The evidence of the driver (F18 design 4.5): an evidence the machine returns or a peer sends is
// checked against the chain at the height of the consensus, sent on once, and submitted in the
// transaction @1BftEvidence, signed with the key of the node, once the chain has its height.

const (
	// bftEvidences is the evidence the driver remembers and the evidence it keeps to submit
	bftEvidences = 1024
	// bftOverdueEvery is how often the driver logs a review past its deadline again
	bftOverdueEvery = 24 * time.Hour
)

// bftEvidence is the evidence the driver took; the handler of the service also uses it
type bftEvidence struct {
	mu      sync.Mutex
	seen    map[bft.EvidenceKey]bool
	order   []bft.EvidenceKey
	pending [][]byte
}

// take records the evidence of a key, and reports whether it is new; past bftEvidences the
// oldest is forgotten, and past bftEvidences waiting the evidence is sent on but not submitted
func (s *bftEvidence) take(key bft.EvidenceKey, body []byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen[key] {
		return false
	}
	if s.seen == nil {
		s.seen = map[bft.EvidenceKey]bool{}
	}
	if len(s.order) == bftEvidences {
		delete(s.seen, s.order[0])
		s.order = s.order[1:]
	}
	s.seen[key] = true
	s.order = append(s.order, key)
	if len(s.pending) < bftEvidences {
		s.pending = append(s.pending, body)
	}
	return true
}

// takeEvidence checks an evidence against the chain at the height of the consensus, whose
// validators set is stored, and keeps a new one to submit; it reports whether the evidence is
// new and valid, to send it on
func (d *bftDriver) takeEvidence(body []byte) bool {
	last, err := lastBlock()
	if err != nil {
		d.logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting the last block")
		return false
	}
	e, key, err := bft.CheckEvidence(body, d.networkID, smart.EvidenceChain{Top: last + 1})
	if err != nil {
		d.logger.WithFields(log.Fields{"type": consts.InvalidObject, "error": err}).Debug("an evidence refused")
		return false
	}
	if !d.evidence.take(key, body) {
		return false
	}
	d.logger.WithFields(log.Fields{"type": consts.BlockError, "height": e.Height, "round": e.Round,
		"kind": e.Kind, "validator": e.Validator}).Warn("evidence of a conflicting message")
	return true
}

// submitEvidence submits the evidence of the heights the chain has, each still valid against the
// chain as a block after the last one checks it; the others wait for their height
func (d *bftDriver) submitEvidence() {
	d.evidence.mu.Lock()
	pending := d.evidence.pending
	d.evidence.mu.Unlock()
	if len(pending) == 0 {
		return
	}
	last, err := lastBlock()
	if err != nil {
		d.logger.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting the last block")
		return
	}
	var kept [][]byte
	for _, body := range pending {
		e, err := bft.UnmarshalEvidence(body, d.networkID)
		if err == nil && e.Height > last {
			kept = append(kept, body)
			continue
		}
		if _, _, err := bft.CheckEvidence(body, d.networkID, smart.EvidenceChain{Top: last}); err != nil {
			d.logger.WithFields(log.Fields{"type": consts.InvalidObject, "error": err}).Debug("an evidence not submitted")
			continue
		}
		if err := d.submit(body); err != nil {
			d.logger.WithFields(log.Fields{"type": consts.ContractError, "error": err}).Error("submitting an evidence")
			kept = append(kept, body)
			continue
		}
		d.logger.WithFields(log.Fields{"height": e.Height, "round": e.Round, "validator": e.Validator}).Info("submitted an evidence")
	}
	d.evidence.mu.Lock()
	d.evidence.pending = append(kept, d.evidence.pending[len(pending):]...)
	d.evidence.mu.Unlock()
}

// errNotValidator is the error of a node out of honor_nodes, which signs no evidence transaction
var errNotValidator = errors.New("the node is not in honor_nodes")

// submitEvidenceTx submits an evidence in the transaction @1BftEvidence, signed with the key of
// the node, whose account is the key and the signer of the transaction: a node of honor_nodes,
// stopped or not, signs it, for no fee
func submitEvidenceTx(body []byte) error {
	pub := syspar.GetNodePubKey()
	if !slices.ContainsFunc(syspar.GetNodes(), func(n syspar.HonorNode) bool { return bytes.Equal(n.PublicKey, pub) }) {
		return errNotValidator
	}
	contract := smart.VMGetContract(script.GetVM(), "BftEvidence", 1)
	if contract == nil {
		return fmt.Errorf("no contract %s", smart.BftEvidenceContract)
	}
	sc := types.SmartTransaction{
		Header: &types.Header{
			ID:          int(contract.Info().ID),
			EcosystemID: 1,
			KeyID:       crypto.Address(pub),
			Time:        time.Now().Unix(),
			NetworkID:   conf.Config.LocalConf.NetworkID,
		},
		Params: map[string]any{"Evidence": body},
	}
	stp := &transaction.SmartTransactionParser{
		SmartContract: &smart.SmartContract{TxSmart: new(types.SmartTransaction)},
	}
	txData, err := stp.BinMarshalWithPrivate(&sc, syspar.GetNodePrivKey(), true)
	if err != nil {
		return err
	}
	return transaction.CreateTransaction(txData, stp.Hash, sc.KeyID, stp.Timestamp)
}
