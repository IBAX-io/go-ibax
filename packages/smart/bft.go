/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package smart

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/IBAX-io/go-ibax/packages/bft"
	"github.com/IBAX-io/go-ibax/packages/conf"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	log "github.com/sirupsen/logrus"
)

// The contracts of the evidence and the reviews of a chain of the BFT consensus (F18 design 4.5).
// A validator signs BftEvidenceContract with the key of its node, for no fee; the vote of a review
// runs BftReviewConcludeContract, from VotingDecisionCheck.
const (
	BftEvidenceContract       = "@1BftEvidence"
	BftReviewConcludeContract = "@1BftReviewConclude"
	votingDecisionCheck       = "@1VotingDecisionCheck"

	// The conclusions of a review
	BftReviewMaintain = "maintain"
	BftReviewRestore  = "restore"

	bftDay = 86400
)

var (
	errBftNotConsensus = errors.New("the chain is not of the BFT consensus")
	errBftCaller       = errors.New("the function runs only in its contract")
	errBftBlockFull    = errors.New("the block has an evidence for each validator")
	errBftReview       = errors.New("the review is not open")
	errBftConclusion   = errors.New("a review concludes maintain, or restore with its reason and version")
)

// BftValidatorsOf is the validators set of a height as the database of the transaction has it
// (design 4.1): the nodes of honor_nodes neither stopped nor out by an evidence at the height
func BftValidatorsOf(dbTx *sqldb.DbTransaction, height int64) (bft.Validators, error) {
	nodes, err := bftHonorNodes(dbTx)
	if err != nil {
		return bft.Validators{}, err
	}
	out, err := bftOut(dbTx, height)
	if err != nil {
		return bft.Validators{}, err
	}
	var set bft.Validators
	for _, n := range nodes {
		if !n.Stopped && !out(n.PublicKey) {
			set.Keys = append(set.Keys, n.PublicKey)
		}
	}
	return set, nil
}

// bftHonorNodes is honor_nodes as the database of the transaction has it
func bftHonorNodes(dbTx *sqldb.DbTransaction) ([]*syspar.HonorNode, error) {
	par := &sqldb.PlatformParameter{}
	if _, err := par.Get(dbTx, syspar.HonorNodes); err != nil {
		return nil, err
	}
	var nodes []*syspar.HonorNode
	if err := json.Unmarshal([]byte(par.Value), &nodes); err != nil {
		return nil, fmt.Errorf("honor_nodes: %w", err)
	}
	return nodes, nil
}

// bftOut reports whether an evidence puts a node out of the set of a height
func bftOut(dbTx *sqldb.DbTransaction, height int64) (func(publicKey []byte) bool, error) {
	stopped, err := (&sqldb.BftReview{}).GetStopped(dbTx, height)
	if err != nil {
		return nil, err
	}
	return func(publicKey []byte) bool {
		return slices.ContainsFunc(stopped, func(r sqldb.BftReview) bool { return bytes.Equal(r.PublicKey, publicKey) })
	}, nil
}

// EvidenceChain is the chain as the checks of an evidence read it (bft.EvidenceChain): its last
// committed block is top, its validators sets are the stored ones
type EvidenceChain struct {
	DbTx *sqldb.DbTransaction
	Top  uint64
}

// Height is bft.EvidenceChain.Height
func (c EvidenceChain) Height() uint64 { return c.Top }

// MaxAge is bft.EvidenceChain.MaxAge
func (c EvidenceChain) MaxAge() uint64 { return syspar.GetBftEvidenceMaxAge() }

// Validators is bft.EvidenceChain.Validators
func (c EvidenceChain) Validators(height uint64) (bft.Validators, error) {
	s := &sqldb.BftValidatorSet{}
	found, err := s.GetAt(c.DbTx, int64(height))
	if err != nil {
		return bft.Validators{}, err
	}
	if !found {
		return bft.Validators{}, fmt.Errorf("no validators set of height %d", height)
	}
	return bft.UnmarshalValidators(s.Data)
}

// HasEvidence is bft.EvidenceChain.HasEvidence
func (c EvidenceChain) HasEvidence(key bft.EvidenceKey) (bool, error) {
	return (&sqldb.BftEvidence{}).Exists(c.DbTx, []byte(key.PublicKey), int64(key.Height), int64(key.Round), int64(key.Kind))
}

// bftRunsIn checks that the chain is of the BFT consensus and that the function runs in the
// contract name, called by the contract caller if it is not empty
func bftRunsIn(sc *SmartContract, name, caller string) error {
	if !syspar.IsBftConsensus() {
		return errBftNotConsensus
	}
	stack := sc.TxContract.StackCont
	if len(stack) == 0 || stack[len(stack)-1] != name {
		return fmt.Errorf("%w: %s", errBftCaller, name)
	}
	if caller != "" && (len(stack) < 2 || stack[len(stack)-2] != caller) {
		return fmt.Errorf("%w: %s from %s", errBftCaller, name, caller)
	}
	return nil
}

// BftApplyEvidence takes the body of an evidence transaction in the contract BftEvidenceContract
// (design 4.5): it makes the six checks of the evidence against the chain before the block,
// refuses an evidence past one for each validator in the block, and writes the evidence and the
// review of the stop of its validator. The validator is out of the set from the next height; the
// review is overdue bft_stop_review_days after the block, and keeps the size and the quorum of the
// set it leaves. It returns the id of the review.
func BftApplyEvidence(sc *SmartContract, body []byte) (int64, error) {
	if err := bftRunsIn(sc, BftEvidenceContract, ""); err != nil {
		return 0, logErrorShort(err, consts.AccessDenied)
	}
	if sc.TxSmart.SignedBy == 0 {
		return 0, logErrorShort(errors.New("a validator signs an evidence with the key of its node"), consts.AccessDenied)
	}
	if sc.BlockHeader == nil {
		return 0, logErrorShort(errors.New("an evidence runs in a block"), consts.InvalidObject)
	}
	block := sc.BlockHeader.BlockId
	e, key, err := bft.CheckEvidence(body, uint64(conf.Config.LocalConf.NetworkID), EvidenceChain{DbTx: sc.DbTransaction, Top: uint64(block - 1)})
	if err != nil {
		return 0, logErrorShort(err, consts.InvalidObject)
	}
	set, err := BftValidatorsOf(sc.DbTransaction, block)
	if err != nil {
		return 0, logErrorDB(err, "the validators set of the block")
	}
	var inBlock int64
	if err := sqldb.GetDB(sc.DbTransaction).Model(&sqldb.BftEvidence{}).Where("block_id = ?", block).Count(&inBlock).Error; err != nil {
		return 0, logErrorDB(err, "counting the evidence of the block")
	}
	if inBlock >= int64(set.N()) {
		return 0, logErrorShort(errBftBlockFull, consts.InvalidObject)
	}
	_, id, err := sc.insert([]string{"public_key", "height", "round", "kind", "data", "block_id"},
		[]any{[]byte(key.PublicKey), int64(e.Height), int64(e.Round), int64(e.Kind), body, block}, "1_bft_evidence")
	if err != nil {
		return 0, err
	}
	stop := block + 1
	next, err := BftValidatorsOf(sc.DbTransaction, stop)
	if err != nil {
		return 0, logErrorDB(err, "the validators set of the next height")
	}
	if _, ok := next.Index([]byte(key.PublicKey)); ok {
		next.Keys = slices.DeleteFunc(next.Keys, func(k []byte) bool { return bytes.Equal(k, []byte(key.PublicKey)) })
	}
	_, review, err := sc.insert([]string{"evidence_id", "public_key", "stop_height", "deadline", "status", "validators", "quorum"},
		[]any{id, []byte(key.PublicKey), stop, sc.blockTime() + syspar.GetBftStopReviewDays()*bftDay, sqldb.BftReviewOpen,
			int64(next.N()), int64(bft.Quorum(next.N()))}, "1_bft_reviews")
	if err != nil {
		return 0, err
	}
	// the driver logs the alert of the stop once the block is committed: a trial of the block
	// runs this too
	sc.GetLogger().WithFields(log.Fields{"review": review, "height": e.Height, "round": e.Round, "kind": e.Kind,
		"stop_height": stop, "validators": next.N()}).Debug("an evidence stops a validator")
	return converter.StrToInt64(review), nil
}

// BftConcludeReview concludes an open review in the contract BftReviewConcludeContract, which the
// vote of the review runs from VotingDecisionCheck (design 4.5). maintain keeps the node out and
// removes it from honor_nodes; restore puts it back in the set from the next height, with the
// reason and the fixed version.
func BftConcludeReview(sc *SmartContract, id int64, conclusion, reason, version string) error {
	if err := bftRunsIn(sc, BftReviewConcludeContract, votingDecisionCheck); err != nil {
		return logErrorShort(err, consts.AccessDenied)
	}
	if sc.BlockHeader == nil {
		return logErrorShort(errors.New("a review concludes in a block"), consts.InvalidObject)
	}
	r := &sqldb.BftReview{}
	found, err := r.Get(sc.DbTransaction, id)
	if err != nil {
		return logErrorDB(err, "getting the review")
	}
	if !found || r.Status != sqldb.BftReviewOpen {
		return logErrorShort(fmt.Errorf("%w: %d", errBftReview, id), consts.InvalidObject)
	}
	block := sc.BlockHeader.BlockId
	fields, values := []string{"decided_block", "reason"}, []any{block, reason}
	switch {
	case conclusion == BftReviewMaintain:
		if err := sc.removeHonorNode(r.PublicKey); err != nil {
			return err
		}
		fields, values = append(fields, "status"), append(values, sqldb.BftReviewMaintained)
	case conclusion == BftReviewRestore && reason != "" && version != "":
		fields = append(fields, "status", "version", "restore_height")
		values = append(values, sqldb.BftReviewRestored, version, block+1)
	default:
		return logErrorShort(errBftConclusion, consts.InvalidObject)
	}
	_, _, err = sc.update(fields, values, "1_bft_reviews", "id", id)
	return err
}

// removeHonorNode removes the node of a public key from honor_nodes, if it is there. The other
// nodes keep their JSON as it is: HonorNode does not marshal every field of a node (key_id, stopped).
func (sc *SmartContract) removeHonorNode(publicKey []byte) error {
	par := &sqldb.PlatformParameter{}
	if _, err := par.Get(sc.DbTransaction, syspar.HonorNodes); err != nil {
		return logErrorDB(err, "getting honor_nodes")
	}
	var raws []json.RawMessage
	if err := json.Unmarshal([]byte(par.Value), &raws); err != nil {
		return logError(err, consts.JSONUnmarshallError, "unmarshalling honor_nodes")
	}
	kept := raws[:0:0]
	for _, raw := range raws {
		n := &syspar.HonorNode{}
		if err := json.Unmarshal(raw, n); err != nil {
			return logError(err, consts.JSONUnmarshallError, "unmarshalling a node of honor_nodes")
		}
		if !bytes.Equal(n.PublicKey, publicKey) {
			kept = append(kept, raw)
		}
	}
	if len(kept) == len(raws) {
		return nil
	}
	value, err := json.Marshal(kept)
	if err != nil {
		return logError(err, consts.JSONMarshallError, "marshalling honor_nodes")
	}
	return sc.setHonorNodes(par, string(value))
}
