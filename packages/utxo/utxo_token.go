package utxo

import (
	"context"

	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
)

var (
	outputsMap map[int64][]sqldb.SpentInfo
	ctx        context.Context
)

const CONTEXT = "CONTEXT"

type Context struct {
	KeyID     int64
	TxOutputs []sqldb.SpentInfo
	TxInputs  []sqldb.SpentInfo
}

func init() {
	var _context Context
	ctx = context.WithValue(context.Background(), CONTEXT, _context)
}

func GetContext() Context {
	return ctx.Value(CONTEXT).(Context)
}

func SetContext(_context Context) {
	ctx = context.WithValue(ctx, CONTEXT, _context)
}

func GetUnusedOutputs(keyID int64) []sqldb.SpentInfo {

	//outputsMap2[keyID].mu.Lock()
	//defer outputsMap2[keyID].mu.Unlock()

	spentInfos := outputsMap[keyID]
	var inputIndex int32 = 0
	var list []sqldb.SpentInfo
	for _, output := range spentInfos {
		//if &output.InputTxHash == nil {
		if len(output.InputTxHash) == 0 {
			output.InputIndex = inputIndex
			inputIndex++
			list = append(list, output)
		}
	}
	return list
}

func GetAllOutputs(keyID int64) []sqldb.SpentInfo {
	spentInfos := outputsMap[keyID]
	return spentInfos
}

func PutAllOutputs(outputs []sqldb.SpentInfo) {
	for _, output := range outputs {
		spentInfos := outputsMap[output.OutputKeyId]
		spentInfos = append(spentInfos, output)
		PutOutputs(output.OutputKeyId, spentInfos)
	}
}
func PutOutputs(keyID int64, outputs []sqldb.SpentInfo) {
	outputsMap[keyID] = outputs
}
func RemoveAllOutputs() {
	outputsMap = map[int64][]sqldb.SpentInfo{}
}

func getAllOutputs() []sqldb.SpentInfo {
	var list []sqldb.SpentInfo
	for _, outputs := range outputsMap {
		list = append(list, outputs...)
	}
	return list
}
