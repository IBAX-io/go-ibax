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

func InsertTxOutputs(outputTxHash []byte, txOutputsCtx []SpentInfo, outputsMap map[KeyUTXO][]SpentInfo) {
	lock.Lock()
	defer lock.Unlock()
	for _, txOutput := range txOutputsCtx {
		keyUTXO := KeyUTXO{Ecosystem: txOutput.Ecosystem, KeyId: txOutput.OutputKeyId}
		spentInfos := outputsMap[keyUTXO]
		txOutput.OutputTxHash = outputTxHash
		spentInfos = append(spentInfos, txOutput)
		outputsMap[keyUTXO] = spentInfos
	}
}
func InsertTxOutputsChange(outputTxHash []byte, inputChange SpentInfo, txOutputsCtx []SpentInfo, outputsMap map[KeyUTXO][]SpentInfo) {
	lock.Lock()
	defer lock.Unlock()
	keyUTXO := KeyUTXO{Ecosystem: inputChange.Ecosystem, KeyId: inputChange.OutputKeyId}
	spentInfosChange := outputsMap[keyUTXO]
	var outputIndex int32
	for index, info := range spentInfosChange {
		if strings.EqualFold(info.Action, "change") {
			spentInfosChange = append(spentInfosChange[:index], spentInfosChange[index+1:]...) // delete change
			outputIndex = info.OutputIndex
			break
		}
	}
	outputsMap[keyUTXO] = spentInfosChange

	for _, txOutput := range txOutputsCtx {
		txKeyUTXO := KeyUTXO{Ecosystem: txOutput.Ecosystem, KeyId: txOutput.OutputKeyId}
		spentInfos := outputsMap[txKeyUTXO]
		txOutput.OutputTxHash = outputTxHash
		txOutput.OutputIndex = outputIndex
		outputIndex++
		spentInfos = append(spentInfos, txOutput)
		outputsMap[txKeyUTXO] = spentInfos
	}
}

func UpdateTxInputs(inputTxHash []byte, txInputsCtx []SpentInfo, outputsMap map[KeyUTXO][]SpentInfo) {
	lock.Lock()
	defer lock.Unlock()
	var inputIndex int32
	for _, txInput := range txInputsCtx {
		// spentInfos := GetUnusedOutputsMap(outputsMap, txInput.OutputKeyId)
		txKeyUTXO := KeyUTXO{Ecosystem: txInput.Ecosystem, KeyId: txInput.OutputKeyId}
		spentInfos := outputsMap[txKeyUTXO]
		for i, info := range spentInfos {
			if bytes.EqualFold(info.OutputTxHash, txInput.OutputTxHash) &&
				info.OutputKeyId == txInput.OutputKeyId &&
				info.OutputIndex == txInput.OutputIndex &&
				len(txInput.InputTxHash) == 0 && len(info.InputTxHash) == 0 {
				outputsMap[txKeyUTXO][i].InputTxHash = inputTxHash
				outputsMap[txKeyUTXO][i].InputIndex = inputIndex
				inputIndex++
			}
		}
	}
}

func PutAllOutputsMap(outputs []SpentInfo, outputsMap map[KeyUTXO][]SpentInfo) {
	lock.Lock()
	defer lock.Unlock()
	//if len(outputsMap) == 0 {
	//	outputsMap = make(map[KeyUTXO][]SpentInfo)
	//}
	for _, output := range outputs {
		keyUTXO := KeyUTXO{Ecosystem: output.Ecosystem, KeyId: output.OutputKeyId}
		spentInfos := outputsMap[keyUTXO]
		spentInfos = append(spentInfos, output)

		PutOutputsMap(keyUTXO, spentInfos, outputsMap)
	}
}
func PutOutputsMap(keyUTXO KeyUTXO, outputs []SpentInfo, outputsMap map[KeyUTXO][]SpentInfo) {
	outputsMap[keyUTXO] = outputs
}

func GetChangeOutputsMap(keyUTXO KeyUTXO, outputsMap map[KeyUTXO][]SpentInfo) *SpentInfo {
	lock.Lock()
	defer lock.Unlock()
	spentInfos := outputsMap[keyUTXO]
	for _, info := range spentInfos {
		if strings.EqualFold(info.Action, "change") {
			return &info
		}
	}
	return nil
}
func GetUnusedOutputsMap(keyUTXO KeyUTXO, outputsMap map[KeyUTXO][]SpentInfo) []SpentInfo {
	lock.Lock()
	defer lock.Unlock()
	spentInfos := outputsMap[keyUTXO]
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
func GetBalanceOutputsMap(keyUTXO KeyUTXO, outputsMap map[KeyUTXO][]SpentInfo) *decimal.Decimal {
	txInputs := GetUnusedOutputsMap(keyUTXO, outputsMap)
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

func GetAllOutputs(outputsMap map[KeyUTXO][]SpentInfo) []SpentInfo {
	var list []SpentInfo
	for _, outputs := range outputsMap {
		list = append(list, outputs...)
	}
	outputsMap = make(map[KeyUTXO][]SpentInfo)
	return list
}
