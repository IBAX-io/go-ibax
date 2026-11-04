/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The tables of a chain of the BFT consensus (F18). bft_certificates, bft_set_proofs and
// bft_sign_state are of the node: a committed block is never rolled back on such a chain, so a
// node writes them as it commits and keeps them. bft_validator_sets, 1_bft_evidence and
// 1_bft_reviews are chain state, written in the transactions of the blocks, the evidences and the
// reviews and rolled back with them.

// BftCertificate is the commit certificate of a height: Data is bft.Certificate.Marshal
type BftCertificate struct {
	Height int64 `gorm:"primary_key;not null"`
	Round  int64 `gorm:"not null"`
	Hash   []byte
	Data   []byte
}

// TableName returns name of table
func (BftCertificate) TableName() string {
	return "bft_certificates"
}

// Create stores the certificate in the transaction of its block
func (c *BftCertificate) Create(dbTx *DbTransaction) error {
	return GetDB(dbTx).Create(c).Error
}

// Get reads the certificate of a height
func (c *BftCertificate) Get(dbTx *DbTransaction, height int64) (bool, error) {
	return isFound(GetDB(dbTx).Where("height = ?", height).First(c))
}

// GetArchived reads the certificate of a height for the synchronization of other nodes: on an
// archive node, out of the window too
func (c *BftCertificate) GetArchived(height int64) (bool, error) {
	return isFound(archived(c.TableName()).Where("height = ?", height).First(c))
}

// BftValidatorSet is the set of validators from a height on until the next row: Data is
// bft.Validators.Marshal
type BftValidatorSet struct {
	FromHeight int64 `gorm:"primary_key;not null"`
	Data       []byte
}

// TableName returns name of table
func (BftValidatorSet) TableName() string {
	return "bft_validator_sets"
}

// Create stores the set that starts at FromHeight
func (s *BftValidatorSet) Create(dbTx *DbTransaction) error {
	return GetDB(dbTx).Create(s).Error
}

// GetAt reads the set of a height: the row of the greatest FromHeight not above it
func (s *BftValidatorSet) GetAt(dbTx *DbTransaction, height int64) (bool, error) {
	return isFound(GetDB(dbTx).Where("from_height <= ?", height).Order("from_height desc").First(s))
}

// BftSetProof is the proof of a change of the validators set out of the window of the blocks: the
// block of Height, whose header has the hash of the set of Height+1, and its certificate. A node
// that joins from a snapshot walks the sets from block 1 to the snapshot height by the proofs
// (design 4.5).
type BftSetProof struct {
	Height int64 `gorm:"primary_key;not null"`
	Block  []byte
	Cert   []byte
}

// TableName returns name of table
func (BftSetProof) TableName() string {
	return "bft_set_proofs"
}

// SetProof is the proof of the validators set from a height on: the block before the height and
// its certificate, and the set
type SetProof struct {
	FromHeight int64
	Block      []byte
	Cert       []byte
	Set        []byte
}

// SetProofs reads up to n proofs of the sets from the heights after from, ascending: of the
// blocks the node keeps, and of those out of its window, bft_set_proofs. Block 1 has no
// certificate, so the set it leaves has no proof: a node has it of its block 1.
func SetProofs(from int64, n int) ([]SetProof, error) {
	var ps []SetProof
	err := DBConn.Raw(fmt.Sprintf(`SELECT DISTINCT ON ("from_height") * FROM (
			SELECT s."from_height", b."data" AS "block", c."data" AS "cert", s."data" AS "set"
			FROM "bft_validator_sets" s
			JOIN %s b ON b."id" = s."from_height" - 1
			JOIN %s c ON c."height" = s."from_height" - 1
			WHERE s."from_height" > ?
			UNION ALL
			SELECT s."from_height", p."block", p."cert", s."data"
			FROM "bft_validator_sets" s JOIN "bft_set_proofs" p ON p."height" = s."from_height" - 1
			WHERE s."from_height" > ?
		) AS proofs ORDER BY "from_height" LIMIT ?`, ArchivedTable("block_chain"), ArchivedTable("bft_certificates")),
		from, from, n).Scan(&ps).Error
	return ps, err
}

// SaveSetProofs stores proofs of the sets out of the window of the blocks
func SaveSetProofs(dbTx *DbTransaction, ps []BftSetProof) error {
	if len(ps) == 0 {
		return nil
	}
	return GetDB(dbTx).Clauses(clause.OnConflict{DoNothing: true}).Create(&ps).Error
}

// SnapshotJoin is the snapshot a node joined the chain from (design 4.5): the node has no
// rollback records before its height, nor the partitions the boundary before it retired
type SnapshotJoin struct {
	Height int64 `gorm:"primary_key;not null"`
	Hash   []byte
}

// TableName returns name of table
func (SnapshotJoin) TableName() string {
	return "snapshot_joins"
}

// Create records the join
func (j *SnapshotJoin) Create(dbTx *DbTransaction) error {
	return GetDB(dbTx).Create(j).Error
}

// JoinedHeight is the height of the last snapshot the node joined from, 0 if none
func JoinedHeight(dbTx *DbTransaction) (int64, error) {
	var h []int64
	if err := GetDB(dbTx).Raw(`SELECT coalesce(max("height"), 0) FROM "snapshot_joins"`).Scan(&h).Error; err != nil {
		return 0, err
	}
	if len(h) == 0 {
		return 0, nil
	}
	return h[0], nil
}

// BftStateHash is the state hash this node computed of the snapshot of a height: the hash of the
// state as of the commit of its block (packages/snapshot)
type BftStateHash struct {
	Height int64 `gorm:"primary_key;not null"`
	Hash   []byte
}

// TableName returns name of table
func (BftStateHash) TableName() string {
	return "bft_state_hashes"
}

// Save writes the hash of its height, outside any block transaction
func (h *BftStateHash) Save() error {
	return DBConn.Save(h).Error
}

// SaveIn writes the hash of its height in a transaction
func (h *BftStateHash) SaveIn(dbTx *DbTransaction) error {
	return GetDB(dbTx).Save(h).Error
}

// Get reads the hash of a height
func (h *BftStateHash) Get(height int64) (bool, error) {
	return isFound(DBConn.Where("height = ?", height).First(h))
}

// BftSignState is what a validator signed at its current height, with its lock and valid value,
// written before the node sends a signed message so that a restart does not sign a conflicting
// one
type BftSignState struct {
	Height int64 `gorm:"primary_key;not null"`
	Data   []byte
}

// TableName returns name of table
func (BftSignState) TableName() string {
	return "bft_sign_state"
}

// Save writes the state of its height and drops the states of the heights below, outside any
// block transaction
func (s *BftSignState) Save() error {
	return DBConn.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(s).Error; err != nil {
			return err
		}
		return tx.Where("height < ?", s.Height).Delete(&BftSignState{}).Error
	})
}

// GetLast reads the state of the highest height
func (s *BftSignState) GetLast() (bool, error) {
	return isFound(DBConn.Order("height desc").First(s))
}

// BftEvidence is an evidence on the chain, unique by its key (public_key, height, round, kind),
// of the kinds bft.KindDoubleProposal and after; Data is the body of its transaction and BlockID the block that carries it
type BftEvidence struct {
	ID        int64
	PublicKey []byte
	Height    int64
	Round     int64
	Kind      int64
	Data      []byte
	BlockID   int64
}

// TableName returns name of table
func (BftEvidence) TableName() string {
	return "1_bft_evidence"
}

// Exists reports whether an evidence of the key is on the chain
func (e *BftEvidence) Exists(dbTx *DbTransaction, publicKey []byte, height, round, kind int64) (bool, error) {
	return isFound(GetDB(dbTx).Where("public_key = ? AND height = ? AND round = ? AND kind = ?",
		publicKey, height, round, kind).First(e))
}

// The statuses of a review
const (
	// BftReviewOpen is a review with no conclusion yet; the node is stopped
	BftReviewOpen int64 = 0
	// BftReviewMaintained keeps the node stopped; a vote removes it from honor_nodes
	BftReviewMaintained int64 = 1
	// BftReviewRestored puts the node back in the set from RestoreHeight on
	BftReviewRestored int64 = 2
)

// BftReview is the review of the stop of a node by an evidence. The node is out of the set from
// StopHeight on, and back from RestoreHeight on when the review restores it. Deadline is the unix
// time the review is overdue after; DecidedBlock is the block of the conclusion, Reason and
// Version the cause and the fixed version of a restore. Validators and Quorum are the size and the
// quorum of the set the stop leaves: the record of the stop is the record of its event.
type BftReview struct {
	ID            int64
	EvidenceID    int64
	PublicKey     []byte
	StopHeight    int64
	Deadline      int64
	Status        int64
	Reason        string
	Version       string
	DecidedBlock  int64
	RestoreHeight int64
	Validators    int64
	Quorum        int64
}

// TableName returns name of table
func (BftReview) TableName() string {
	return "1_bft_reviews"
}

// Get reads the review of an id
func (r *BftReview) Get(dbTx *DbTransaction, id int64) (bool, error) {
	return isFound(GetDB(dbTx).Where("id = ?", id).First(r))
}

// GetStopped reads the reviews that keep a node out of the set of a height
func (r *BftReview) GetStopped(dbTx *DbTransaction, height int64) ([]BftReview, error) {
	var rs []BftReview
	err := GetDB(dbTx).Where("stop_height <= ? AND (restore_height = 0 OR restore_height > ?)", height, height).
		Order("id").Find(&rs).Error
	return rs, err
}

// GetStoppedIn reads the reviews of the nodes an evidence stops from a height after from and up to
// to
func (r *BftReview) GetStoppedIn(dbTx *DbTransaction, from, to int64) ([]BftReview, error) {
	var rs []BftReview
	err := GetDB(dbTx).Where("stop_height > ? AND stop_height <= ?", from, to).Order("id").Find(&rs).Error
	return rs, err
}

// BftRounds reads how many of the certificates of the heights after from were committed at each
// round
func BftRounds(from int64) (map[int64]int64, error) {
	var rows []struct{ Round, Count int64 }
	if err := DBConn.Model(&BftCertificate{}).Select("round, count(*) AS count").Where("height > ?", from).
		Group("round").Scan(&rows).Error; err != nil {
		return nil, err
	}
	rounds := map[int64]int64{}
	for _, r := range rows {
		rounds[r.Round] = r.Count
	}
	return rounds, nil
}

// GetOverdue reads the open reviews past their deadline at the unix time now
func (r *BftReview) GetOverdue(dbTx *DbTransaction, now int64) ([]BftReview, error) {
	var rs []BftReview
	err := GetDB(dbTx).Where("status = ? AND deadline < ?", BftReviewOpen, now).Order("id").Find(&rs).Error
	return rs, err
}
