package sqldb

import (
	"bytes"

	"github.com/shopspring/decimal"
)

var (
	outputsMap map[int64][]SpentInfo
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

func InsertTxOutputs(outputTxHash []byte, txOutputsCtx []SpentInfo) {
	for _, txOutput := range txOutputsCtx {
		spentInfos := outputsMap[txOutput.OutputKeyId]
		txOutput.OutputTxHash = outputTxHash
		// txOutput.Height=height
		spentInfos = append(spentInfos, txOutput)
		outputsMap[txOutput.OutputKeyId] = spentInfos
	}
}

func UpdateTxInputs(inputTxHash []byte, txInputsCtx []SpentInfo) {
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

func PutAllOutputsMap(outputs []SpentInfo) {
	outputsMap = make(map[int64][]SpentInfo)
	for _, output := range outputs {
		spentInfos := outputsMap[output.OutputKeyId]
		spentInfos = append(spentInfos, output)
		PutOutputsMap(output.OutputKeyId, spentInfos)
	}
}
func PutOutputsMap(keyID int64, outputs []SpentInfo) {
	outputsMap[keyID] = outputs
}

func GetUnusedOutputsMap(keyID int64) []SpentInfo {
	spentInfos := outputsMap[keyID]
	var inputIndex int32
	var list []SpentInfo
	for _, output := range spentInfos {
		if len(output.InputTxHash) == 0 {
			output.InputIndex = inputIndex
			inputIndex++
			list = append(list, output)
		}
	}
	return list
}
func GetBalance(keyID int64) *decimal.Decimal {
	txInputs := GetUnusedOutputsMap(keyID)
	balance := decimal.Zero
	if len(txInputs) > 0 {
		for _, input := range txInputs {
			outputValue, _ := decimal.NewFromString(input.OutputValue)
			balance = balance.Add(outputValue)
		}
		return &balance
	}
	return nil

}

func GetAllOutputs() []SpentInfo {
	var list []SpentInfo
	for _, outputs := range outputsMap {
		list = append(list, outputs...)
	}
	outputsMap = make(map[int64][]SpentInfo)
	return list
}
