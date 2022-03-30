package query

import (
	"fmt"
	"sync"
)

const maxBlockIDEndpoint = "/api/v2/maxblockid"
const blockInfoEndpoint = "/api/v2/block/%d"

type MaxBlockID struct {
	MaxBlockID int64 `json:"max_block_id"`
}

type blockInfoResult struct {
	BlockID       int64  `json:"block_id"`
	Hash          []byte `json:"hash"`
	EcosystemID   int64  `json:"ecosystem_id"`
	KeyID         int64  `json:"key_id"`
	Time          int64  `json:"time"`
	Tx            int32  `json:"tx_count"`
	RollbacksHash []byte `json:"rollbacks_hash"`
}

func MaxBlockIDs(nodesList []string) ([]int64, error) {
	wg := sync.WaitGroup{}
	workResults := ConcurrentMap{m: map[string]any{}}
	for _, nodeUrl := range nodesList {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			maxBlockID := &MaxBlockID{}
			if err := sendGetRequest(url+maxBlockIDEndpoint, maxBlockID); err != nil {
				workResults.Set(url, err)
				return
			}
			workResults.Set(url, maxBlockID.MaxBlockID)
		}(nodeUrl)
	}
	wg.Wait()
	maxBlockIds := []int64{}
	for _, result := range workResults.m {
		switch res := result.(type) {
		case int64:
			maxBlockIds = append(maxBlockIds, res)
		case error:
			return nil, res
		}
	}
	return maxBlockIds, nil
}

func BlockInfo(nodesList []string, blockID int64) (map[string]*blockInfoResult, error) {
	wg := sync.WaitGroup{}
	workResults := ConcurrentMap{m: map[string]any{}}
	for _, nodeUrl := range nodesList {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			blockInfo := &blockInfoResult{BlockID: blockID}
			if err := sendGetRequest(url+fmt.Sprintf(blockInfoEndpoint, blockID), blockInfo); err != nil {
				workResults.Set(url, err)
				return
			}
			workResults.Set(url, blockInfo)
		}(nodeUrl)
	}
	wg.Wait()
	result := map[string]*blockInfoResult{}
	for nodeUrl, blockInfoOrError := range workResults.m {
		switch res := blockInfoOrError.(type) {
		case error:
			return nil, res
		case *blockInfoResult:
			result[nodeUrl] = res
		}
	}
	return result, nil
}
