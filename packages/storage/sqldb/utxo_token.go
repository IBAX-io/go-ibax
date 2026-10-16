package sqldb

import (
	"sort"
	"sync"
)

var (
	lock = &sync.RWMutex{}
)

func InsertTxOutputs(outputTxHash []byte, txOutputsMapCtx map[KeyUTXO][]SpentInfo, outputsMap map[KeyUTXO][]SpentInfo) {
	lock.Lock()
	defer lock.Unlock()
	for keyUTXO, txOutput := range txOutputsMapCtx {
		spentInfos := outputsMap[keyUTXO]
		for i, _ := range txOutput {
			txOutput[i].OutputTxHash = outputTxHash
		}
		spentInfos = append(spentInfos, txOutput...)
		outputsMap[keyUTXO] = spentInfos
	}
}

func UpdateTxInputs(inputTxHash []byte, txInputsMapCtx map[KeyUTXO][]SpentInfo, outputsMap map[KeyUTXO][]SpentInfo) {
	lock.Lock()
	defer lock.Unlock()
	// the inputs are numbered key by key in the order of ecosystem and key ID, not in the random
	// order of the map, so that every node writes the same input_index
	keys := make([]KeyUTXO, 0, len(txInputsMapCtx))
	for txKeyUTXO := range txInputsMapCtx {
		keys = append(keys, txKeyUTXO)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Ecosystem != keys[j].Ecosystem {
			return keys[i].Ecosystem < keys[j].Ecosystem
		}
		return keys[i].KeyId < keys[j].KeyId
	})
	var inputIndex int32
	for _, txKeyUTXO := range keys {
		spentInfos := outputsMap[txKeyUTXO]
		for i, info := range spentInfos {
			if len(info.InputTxHash) == 0 {
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

func GetAllOutputs(outputsMap map[KeyUTXO][]SpentInfo) []SpentInfo {
	var list []SpentInfo
	for _, outputs := range outputsMap {
		list = append(list, outputs...)
	}
	outputsMap = make(map[KeyUTXO][]SpentInfo)
	return list
}
