package utxo

import (
	"bytes"

	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
)

var (
	outputsMap map[int64][]sqldb.SpentInfo
)

// func CallContract(outputsMap map[int64][]sqldb.SpentInfo, tx Tx) {
//	_txInputs := GetUnusedOutputsMap(outputsMap, tx.KeyID)
//	smartVM := &SmartVM{TxSmart: tx, TxInputs: _txInputs}
//	_, _ = VMCallContract(smartVM)
//	txInputsCtx := smartVM.TxInputs
//	txOutputsCtx := smartVM.TxOutputs
//	if len(txInputsCtx) > 0 && len(txOutputsCtx) > 0 {
//		updateTxInputs(outputsMap, tx.Hash, txInputsCtx)
//		insertTxOutputs(outputsMap, tx.Hash, txOutputsCtx)
//	}
// }

func InsertTxOutputs(outputTxHash []byte, txOutputsCtx []sqldb.SpentInfo) {
	for _, txOutput := range txOutputsCtx {
		spentInfos := outputsMap[txOutput.OutputKeyId]
		txOutput.OutputTxHash = outputTxHash
		// txOutput.Height=height
		spentInfos = append(spentInfos, txOutput)
		outputsMap[txOutput.OutputKeyId] = spentInfos
	}
}

func UpdateTxInputs(inputTxHash []byte, txInputsCtx []sqldb.SpentInfo) {
	var inputIndex int32
	for _, txInput := range txInputsCtx {
		// spentInfos := GetUnusedOutputsMap(outputsMap, txInput.OutputKeyId)
		spentInfos := outputsMap[txInput.OutputKeyId]
		for i, info := range spentInfos {
			if bytes.EqualFold(info.OutputTxHash, txInput.OutputTxHash) &&
				info.OutputKeyId == txInput.OutputKeyId &&
				info.OutputIndex == txInput.OutputIndex &&
				len(txInput.InputTxHash) == 0 && len(info.InputTxHash) == 0 {
				outputsMap[txInput.OutputKeyId][i].InputTxHash = inputTxHash
				outputsMap[txInput.OutputKeyId][i].InputIndex = inputIndex
				inputIndex++
			}
		}
	}
}

func PutAllOutputsMap(outputs []sqldb.SpentInfo) {
	outputsMap = make(map[int64][]sqldb.SpentInfo)
	for _, output := range outputs {
		spentInfos := outputsMap[output.OutputKeyId]
		spentInfos = append(spentInfos, output)
		PutOutputsMap(output.OutputKeyId, spentInfos)
	}
}
func PutOutputsMap(keyID int64, outputs []sqldb.SpentInfo) {
	outputsMap[keyID] = outputs
}

func GetUnusedOutputsMap(keyID int64) []sqldb.SpentInfo {
	spentInfos := outputsMap[keyID]
	var inputIndex int32
	var list []sqldb.SpentInfo
	for _, output := range spentInfos {
		if len(output.InputTxHash) == 0 {
			output.InputIndex = inputIndex
			inputIndex++
			list = append(list, output)
		}
	}
	return list
}

func GetAllOutputs() []sqldb.SpentInfo {
	var list []sqldb.SpentInfo
	for _, outputs := range outputsMap {
		list = append(list, outputs...)
	}
	outputsMap = make(map[int64][]sqldb.SpentInfo)
	return list
}
