package types

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"io"
	math_bits "math/bits"

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

//BlockHeader is a structure of the block's header
type BlockHeader struct {
	BlockID       int64
	Time          int64
	EcosystemID   int64
	KeyID         int64
	NodePosition  int64
	Sign          []byte
	Hash          []byte
	RollbacksHash []byte //differences with before and after in tx modification table
	Version       int
}

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
	return fmt.Sprintf("%d,%x,%s,%d,%d,%d,%d",
		b.BlockID, prev.Hash, mrklRoot, b.Time, b.EcosystemID, b.KeyID, b.NodePosition) +
		blockVer(b, prev)
}

// ForSign from 128 bytes to 512 bytes. Signature of TYPE, BLOCK_ID, PREV_BLOCK_HASH, TIME, WALLET_ID, state_id, MRKL_ROOT
func (b *BlockHeader) ForSign(prev *BlockHeader, mrklRoot []byte) string {
	return fmt.Sprintf("0,%v,%x,%v,%v,%v,%v,%s",
		b.BlockID, prev.Hash, b.Time, b.EcosystemID, b.KeyID, b.NodePosition, mrklRoot) +
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

// BlockData is a structure of the block's
type BlockData struct {
	Header         *BlockHeader
	PrevHeader     *BlockHeader
	MerkleRoot     []byte
	BinData        []byte
	TxFullData     [][]byte
	TxExecutionSql TxExecutionSql
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

func WithTxExecSql(sql TxExecutionSql) BlockDataOption {
	return func(b *BlockData) error {
		b.TxExecutionSql = sql
		return nil
	}
}

func (b BlockData) ForSign() string {
	return b.Header.ForSign(b.PrevHeader, b.MerkleRoot)
}

func (b *BlockData) GetMerkleRoot() []byte {
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
	//if b.Header.BlockID != 1 {
	for i := 0; i < len(b.TxExecutionSql); i++ {
		d := b.TxExecutionSql[i]
		b.TxExecutionSql[i] = DoZlibCompress(d)
	}
	for i := 0; i < len(b.TxFullData); i++ {
		d := b.TxFullData[i]
		b.TxFullData[i] = DoZlibCompress(d)
	}
	b.MerkleRoot = b.GetMerkleRoot()
	signed, err := b.GetSign(key)
	if err != nil {
		return nil, err
	}
	b.Header.Sign = signed
	b.Header.Hash = b.Header.GenHash(b.PrevHeader, b.MerkleRoot)
	//}
	return json.Marshal(&b)
}

func (b *BlockData) UnmarshallBlock(data []byte) error {
	if len(data) == 0 {
		return ErrZeroBlockSize
	}
	if len(data) < minBlockSize {
		return ErrMinBlockSize(len(data), minBlockSize)
	}
	if err := json.Unmarshal(data, &b); err != nil {
		return errors.Wrap(err, "unmarshalling block")
	}
	for i := 0; i < len(b.TxExecutionSql); i++ {
		d := b.TxExecutionSql[i]
		b.TxExecutionSql[i] = DoZlibUnCompress(d)
	}
	for i := 0; i < len(b.TxFullData); i++ {
		d := b.TxFullData[i]
		b.TxFullData[i] = DoZlibUnCompress(d)
	}
	b.BinData = data
	return nil
}

// TxExecutionSql defined contract exec sql for tx DML
type TxExecutionSql [][]byte

func (t *TxExecutionSql) Reset() { *t = TxExecutionSql{} }

func (t TxExecutionSql) Size() (n int) {
	sovBlock := func(x uint64) (n int) {
		return (math_bits.Len64(x|1) + 6) / 7
	}
	if t == nil {
		return 0
	}
	var l int
	for _, b := range t {
		l = len(b)
		n += 1 + l + sovBlock(uint64(l))
	}
	return n
}

type TxExecSqlMap map[string]TxExecutionSql

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

func DoZlibCompress(src []byte) []byte {
	var in bytes.Buffer
	w := zlib.NewWriter(&in)
	w.Write(src)
	w.Close()
	return in.Bytes()
}

func DoZlibUnCompress(compressSrc []byte) []byte {
	b := bytes.NewReader(compressSrc)
	var out bytes.Buffer
	r, _ := zlib.NewReader(b)
	io.Copy(&out, r)
	return out.Bytes()
}
