package sqldb

import (
	"bytes"
	"strings"
	"sync"

	"github.com/shopspring/decimal"
)

var (
	lock = &sync.RWMutex{}
)

func InsertTxOutputs(outputTxHash []byte, txOutputsCtx []SpentInfo, outputsMap map[int64][]SpentInfo) {
	lock.Lock()
	defer lock.Unlock()
	for index, txOutput := range txOutputsCtx {
		spentInfos := outputsMap[txOutput.OutputKeyId]
		txOutput.OutputTxHash = outputTxHash
		txOutput.OutputIndex = int32(index)
		// txOutput.Height=height
		spentInfos = append(spentInfos, txOutput)
		outputsMap[txOutput.OutputKeyId] = spentInfos
	}
}
func InsertTxOutputsChange(outputTxHash []byte, inputChange SpentInfo, txOutputsCtx []SpentInfo, outputsMap map[int64][]SpentInfo) {
	lock.Lock()
	defer lock.Unlock()
	spentInfosChange := outputsMap[inputChange.OutputKeyId]
	var outputIndex int32
	for index, info := range spentInfosChange {
		if strings.EqualFold(info.Action, "change") {
			spentInfosChange = append(spentInfosChange[:index], spentInfosChange[index+1:]...) // delete change
			outputIndex = info.OutputIndex
			break
		}
	}
	outputsMap[inputChange.OutputKeyId] = spentInfosChange

	for _, txOutput := range txOutputsCtx {
		spentInfos := outputsMap[txOutput.OutputKeyId]
		txOutput.OutputTxHash = outputTxHash
		txOutput.OutputIndex = outputIndex
		outputIndex++
		spentInfos = append(spentInfos, txOutput)
		outputsMap[txOutput.OutputKeyId] = spentInfos
	}
}

func UpdateTxInputs(inputTxHash []byte, txInputsCtx []SpentInfo, outputsMap map[int64][]SpentInfo) {
	lock.Lock()
	defer lock.Unlock()
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

func PutAllOutputsMap(outputs []SpentInfo, outputsMap map[int64][]SpentInfo) {
	//lock.Lock()
	//defer lock.Unlock()
	//if len(outputsMap) == 0 {
	//	outputsMap = make(map[int64][]SpentInfo)
	//}
	for _, output := range outputs {
		spentInfos := outputsMap[output.OutputKeyId]
		spentInfos = append(spentInfos, output)
		var ecosystem int64 // TODO ecosystem
		PutOutputsMap(ecosystem, output.OutputKeyId, spentInfos, outputsMap)
	}
}
func PutOutputsMap(ecosystem int64, keyID int64, outputs []SpentInfo, outputsMap map[int64][]SpentInfo) {
	//lock.Lock()
	//defer lock.Unlock()
	outputsMap[keyID] = outputs
}

func GetChangeOutputsMap(keyID int64, txHash []byte, outputsMap map[int64][]SpentInfo) *SpentInfo {
	lock.Lock()
	defer lock.Unlock()
	spentInfos := outputsMap[keyID]
	for _, info := range spentInfos {
		if strings.EqualFold(info.Action, "change") {
			return &info
		}
	}
	return nil
}
func GetUnusedOutputsMap(keyID int64, outputsMap map[int64][]SpentInfo) []SpentInfo {
	lock.Lock()
	defer lock.Unlock()
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
func GetBalanceOutputsMap(ecosystem int64, keyID int64, outputsMap map[int64][]SpentInfo) *decimal.Decimal {
	txInputs := GetUnusedOutputsMap(keyID, outputsMap)
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

func GetAllOutputs(outputsMap map[int64][]SpentInfo) []SpentInfo {
	var list []SpentInfo
	for _, outputs := range outputsMap {
		list = append(list, outputs...)
	}
	outputsMap = make(map[int64][]SpentInfo)
	return list
}
