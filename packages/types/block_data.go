package types

import (
	"bytes"
	"fmt"

	"github.com/gogo/protobuf/proto"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/pkg/errors"
)

const (
	minBlockSize = 9
)

var (
	ErrMaxBlockSize = func(max int64, size int) error {
		return fmt.Errorf("block size exceeds maximum %d limit, size is %d", max, size)
	}
	ErrMinBlockSize = func(min int, size int) error {
		return fmt.Errorf("block size exceeds minimum %d limit, size is %d", min, size)
	}
	ErrZeroBlockSize   = errors.New("Block size is zero")
	ErrUnmarshallBlock = errors.New("Unmarshall block")
)

func blockVer(cur, prev *BlockHeader) (ret string) {
	if cur.Version >= consts.BvRollbackHash {
		ret = fmt.Sprintf(",%x", prev.RollbacksHash)
	}
	return
}

func (b *BlockHeader) GenHash(prev *BlockHeader, mrklRoot []byte) []byte {
	return crypto.DoubleHash([]byte(b.ForSha(prev, mrklRoot)))
}

func (b *BlockHeader) ForSha(prev *BlockHeader, mrklRoot []byte) string {
	return fmt.Sprintf("%d,%x,%s,%d,%d,%d,%d,%d",
		b.BlockId, prev.BlockHash, mrklRoot, b.Timestamp, b.EcosystemId, b.KeyId, b.NodePosition, b.NetworkId) +
		blockVer(b, prev)
}

// ForSign from 128 bytes to 512 bytes. Signature of TYPE, BLOCK_ID, PREV_BLOCK_HASH, TIME, WALLET_ID, state_id, MRKL_ROOT
func (b *BlockHeader) ForSign(prev *BlockHeader, mrklRoot []byte) string {
	return fmt.Sprintf("0,%v,%x,%v,%v,%v,%v,%s,%d",
		b.BlockId, prev.BlockHash, b.Timestamp, b.EcosystemId, b.KeyId, b.NodePosition, mrklRoot, b.NetworkId) +
		blockVer(b, prev)
}

// ParseBlockHeader is parses block header
func ParseBlockHeader(buf *bytes.Buffer, maxBlockSize int64) (header *BlockHeader, err error) {
	if int64(buf.Len()) > maxBlockSize {
		err = ErrMaxBlockSize(maxBlockSize, buf.Len())
		return
	}
	blo := &BlockData{}
	if err := blo.UnmarshallBlock(buf.Bytes()); err != nil {
		return nil, err
	}
	return blo.Header, nil
}

type BlockDataOption func(b *BlockData) error

func (b *BlockData) Apply(opts ...BlockDataOption) error {
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(b); err != nil {
			return err
		}
	}
	return nil
}

func WithCurHeader(cur *BlockHeader) BlockDataOption {
	return func(b *BlockData) error {
		b.Header = cur
		return nil
	}
}

func WithPrevHeader(pre *BlockHeader) BlockDataOption {
	return func(b *BlockData) error {
		b.PrevHeader = pre
		return nil
	}
}

func WithTxFullData(data [][]byte) BlockDataOption {
	return func(b *BlockData) error {
		b.TxFullData = data
		return nil
	}
}

func WithAfterTxs(a *AfterTxs) BlockDataOption {
	return func(b *BlockData) error {
		b.AfterTxs = a
		return nil
	}
}
func WithSysUpdate(a bool) BlockDataOption {
	return func(b *BlockData) error {
		b.SysUpdate = a
		return nil
	}
}

func (b BlockData) ForSign() string {
	return b.Header.ForSign(b.PrevHeader, b.MerkleRoot)
}

func (b *BlockData) GenMerkleRoot() []byte {
	var mrklArray [][]byte
	for _, tr := range b.TxFullData {
		mrklArray = append(mrklArray, converter.BinToHex(crypto.DoubleHash(tr)))
	}
	if len(mrklArray) == 0 {
		mrklArray = append(mrklArray, []byte("0"))
	}
	return MerkleTreeRoot(mrklArray)
}

func (b *BlockData) GetSign(key []byte) ([]byte, error) {
	forSign := b.ForSign()
	signed, err := crypto.Sign(key, []byte(forSign))
	if err != nil {
		return nil, errors.Wrap(err, "signing block")
	}
	return signed, nil
}

// MarshallBlock is marshalling block
func (b *BlockData) MarshallBlock(key []byte) ([]byte, error) {
	//if b.AfterTxs != nil {
	//	for i := 0; i < len(b.AfterTxs.TxBinLogSql); i++ {
	//		b.AfterTxs.TxBinLogSql[i] = DoZlibCompress(b.AfterTxs.TxBinLogSql[i])
	//	}
	//}
	for i := 0; i < len(b.TxFullData); i++ {
		b.TxFullData[i] = DoZlibCompress(b.TxFullData[i])
	}
	b.MerkleRoot = b.GenMerkleRoot()
	signed, err := b.GetSign(key)
	if err != nil {
		return nil, err
	}
	b.Header.Sign = signed
	b.Header.BlockHash = b.Header.GenHash(b.PrevHeader, b.MerkleRoot)
	return proto.Marshal(b)
}

func (b *BlockData) UnmarshallBlock(data []byte) error {
	if len(data) == 0 {
		return ErrZeroBlockSize
	}
	if len(data) < minBlockSize {
		return ErrMinBlockSize(len(data), minBlockSize)
	}
	if err := proto.Unmarshal(data, b); err != nil {
		return errors.Wrap(err, "unmarshalling block")
	}
	//if b.AfterTxs != nil {
	//	for i := 0; i < len(b.AfterTxs.TxBinLogSql); i++ {
	//		b.AfterTxs.TxBinLogSql[i] = DoZlibUnCompress(b.AfterTxs.TxBinLogSql[i])
	//	}
	//}
	for i := 0; i < len(b.TxFullData); i++ {
		b.TxFullData[i] = DoZlibUnCompress(b.TxFullData[i])
	}
	b.BinData = data
	return nil
}

// MerkleTreeRoot return Merkle value
func MerkleTreeRoot(dataArray [][]byte) []byte {
	result := make(map[int32][][]byte)
	for _, v := range dataArray {
		hash := converter.BinToHex(crypto.DoubleHash(v))
		result[0] = append(result[0], hash)
	}
	var j int32
	for len(result[j]) > 1 {
		for i := 0; i < len(result[j]); i = i + 2 {
			if len(result[j]) <= (i + 1) {
				if _, ok := result[j+1]; !ok {
					result[j+1] = [][]byte{result[j][i]}
				} else {
					result[j+1] = append(result[j+1], result[j][i])
				}
			} else {
				if _, ok := result[j+1]; !ok {
					hash := crypto.DoubleHash(append(result[j][i], result[j][i+1]...))
					hash = converter.BinToHex(hash)
					result[j+1] = [][]byte{hash}
				} else {
					hash := crypto.DoubleHash([]byte(append(result[j][i], result[j][i+1]...)))
					hash = converter.BinToHex(hash)
					result[j+1] = append(result[j+1], hash)
				}
			}
		}
		j++
	}

	ret := result[int32(len(result)-1)]
	return ret[0]
}
