package api

import (
	"encoding/json"
	"fmt"
	"github.com/IBAX-io/go-ibax/packages/miner"
	"math/rand"
	"testing"
)

//go test webbench_test.go -test.bench=".*"

func Benchmark_GetMiner(b *testing.B) {
	st, _ := miner.MakeMiningPoolData(100000000)
	//assert.NoError(t, err)
	for i := 0; i < b.N; i++ { //use b.N for looping
		dl := rand.Intn(st)
		miner.GetMiner(dl)
	}
}

func Benchmark_GetMinerTimeConsumingFunction(b *testing.B) {
	b.StopTimer() //

	//
	st, _ := miner.MakeMiningPoolData(100000000)
	b.StartTimer() //
	for i := 0; i < b.N; i++ {
		dl := rand.Intn(st)
		miner.GetMiner(dl)
	}
}

func Benchmark_MapJson(b *testing.B) {
	st, _ := miner.MakeMiningPoolData(100000000)
	//assert.NoError(t, err)
	for i := 0; i < b.N; i++ { //use b.N for looping
		dl := rand.Intn(st)
		miner.GetMiner(dl)
	}
}

const INT64_MAX = int64(^uint64((0)) >> 1)

//const INT_MIN = ^INT_MAX

type AssignRules struct {
	StartBlockID    int64  `json:"start_blockid"`
	EndBlockID      int64  `json:"end_blockid"`
	IntervalBlockID int64  `json:"interval_blockid"`
	Count           int64  `json:"count"`
	TotalAmount     string `json:"total_amount"`
}

func TestMapJson(t *testing.T) {
	//Private placement
	//apri := AssignRules{
	//	StartBlockID:    1,
	//	EndBlockID:      21600*2*365 + 1,
	//	IntervalBlockID: 21600 * 365,
	//	Count:           3,
	//	TotalAmount:     "63000000000000000000",
	//}
	////Public offering
	//apub := AssignRules{
	//	StartBlockID:    1,
	//	EndBlockID:      21600*365 + 21600*4*30 + 1,
	//	IntervalBlockID: 21600 * 4 * 30,
	//	Count:           4,
	//	TotalAmount:     "105000000000000000000",
	//}
	apri := AssignRules{
		StartBlockID:    INT64_MAX,
		EndBlockID:      INT64_MAX,
		IntervalBlockID: 21600 * 360,
		Count:           3,
		TotalAmount:     "63000000000000000000",
	}
	//Public offering
	apub := AssignRules{
		StartBlockID:    INT64_MAX,
		EndBlockID:      INT64_MAX,
		IntervalBlockID: 21600 * 4 * 30,
		Count:           4,
		TotalAmount:     "105000000000000000000",
	}
	//foundation
	ac := AssignRules{
		StartBlockID:    21600*360 + 1,
		EndBlockID:      INT64_MAX,
		IntervalBlockID: 1,
		TotalAmount:     "315000000000000000000",
	}
	//Ecological partner
	ad := AssignRules{
		StartBlockID:    21600*6*30 + 1,
		EndBlockID:      21600*2*360 + 21600*6*30 + 1,
		IntervalBlockID: 21600 * 30,
		Count:           24,
		TotalAmount:     "168000000000000000000",
	}

	//
	//ae :=AssignRules{
	//	StartBlockID:21600*6*30,
	//	EndBlockID:INT64_MAX,
	//	IntervalBlockID:21600*30,
	//	TotalAmount:"105000000000000000000",
	//}
	//team
	af := AssignRules{
		StartBlockID:    21600*6*30 + 1,
		EndBlockID:      21600*30*(48-1) + 21600*6*30 + 1,
		IntervalBlockID: 21600 * 30,
		Count:           48,
		TotalAmount:     "315000000000000000000",
	}

	//mine
	ag := AssignRules{
		StartBlockID:    21600*360 + 1,
		EndBlockID:      INT64_MAX,
		IntervalBlockID: 1,
		TotalAmount:     "1128750000000000000000",
	}

	//founder
	ah := AssignRules{
		StartBlockID:    1,
		EndBlockID:      INT64_MAX,
		IntervalBlockID: 1,
		TotalAmount:     "5250000000000000000",
	}

	ret := make(map[int64]AssignRules, 10)
	ret[1] = apri
	ret[2] = apub
	ret[3] = ac
	ret[4] = ad
	//ret[5] = ae
	ret[5] = af
	ret[6] = ag
	ret[7] = ah
	data, _ := json.Marshal(ret)
	fmt.Println(string(data))

}

func TestMapJsonTs(t *testing.T) {
	//Private placement
	apri := AssignRules{
		StartBlockID:    1,
		EndBlockID:      21,
		IntervalBlockID: 10,
		Count:           3,
		TotalAmount:     "63000000000000000000",
	}
	//Public offering
	apub := AssignRules{
		StartBlockID:    1,
		EndBlockID:      61,
		IntervalBlockID: 20,
		Count:           4,
		TotalAmount:     "105000000000000000000",
	}
	//foundation
	ac := AssignRules{
		StartBlockID:    21600 * 365,
		EndBlockID:      INT64_MAX,
		IntervalBlockID: 1,
		TotalAmount:     "315000000000000000000",
	}
	//Ecological partner
	ad := AssignRules{
		StartBlockID:    1,
		EndBlockID:      49,
		IntervalBlockID: 2,
		Count:           24,
		TotalAmount:     "168000000000000000000",
	}

	//
	//ae :=AssignRules{
	//	StartBlockID:21600*6*30,
	//	EndBlockID:INT64_MAX,
	//	IntervalBlockID:21600*30,
	//	TotalAmount:"105000000000000000000",
	//}
	//team
	af := AssignRules{
		StartBlockID:    1,
		EndBlockID:      48,
		IntervalBlockID: 1,
		Count:           48,
		TotalAmount:     "315000000000000000000",
	}

	//mine
	ag := AssignRules{
		StartBlockID:    21600 * 365,
		EndBlockID:      INT64_MAX,
		IntervalBlockID: 1,
		TotalAmount:     "1128750000000000000000",
	}

	//founder
	ah := AssignRules{
		StartBlockID:    0,
		EndBlockID:      INT64_MAX,
		IntervalBlockID: 1,
		TotalAmount:     "5250000000000000000",
	}

	ret := make(map[int64]AssignRules, 10)
	ret[1] = apri
	ret[2] = apub
	ret[3] = ac
	ret[4] = ad
	//ret[5] = ae
	ret[5] = af
	ret[6] = ag
	ret[7] = ah
	data, _ := json.Marshal(ret)
	fmt.Println(string(data))

}

func TestMapJsonTsFirst(t *testing.T) {
	//Private placement
	apri := AssignRules{
		StartBlockID:    INT64_MAX,
		EndBlockID:      INT64_MAX, //start + 21600 * 2 * 365
		IntervalBlockID: 21600 * 365,
		Count:           3,
		TotalAmount:     "63000000000000000000",
	}
	//Public offering
	apub := AssignRules{
		StartBlockID:    INT64_MAX,
		EndBlockID:      INT64_MAX, //start + 21600 * 365 + 21600 * 4 * 30
		IntervalBlockID: 21600 * 4 * 30,
		Count:           4,
		TotalAmount:     "105000000000000000000",
	}
	//foundation
	ac := AssignRules{
		StartBlockID:    21600 * 365,
		EndBlockID:      INT64_MAX,
		IntervalBlockID: 1,
		TotalAmount:     "315000000000000000000",
	}
	//Ecological partner
	ad := AssignRules{
		StartBlockID:    INT64_MAX,
		EndBlockID:      INT64_MAX, //start + 21600 * 2 * 365
		IntervalBlockID: 21600 * 30,
		Count:           24,
		TotalAmount:     "168000000000000000000",
	}

	//
	//ae :=AssignRules{
	//	StartBlockID:21600*6*30,
	//	EndBlockID:INT64_MAX,
	//	IntervalBlockID:21600*30,
	//	TotalAmount:"105000000000000000000",
	//}
	//team
	af := AssignRules{
		StartBlockID:    INT64_MAX,
		EndBlockID:      INT64_MAX, //start + 21600 * 4 * 365
		IntervalBlockID: 21600 * 30,
		Count:           48,
		TotalAmount:     "315000000000000000000",
	}

	//mine
	ag := AssignRules{
		StartBlockID:    21600 * 365,
		EndBlockID:      INT64_MAX,
		IntervalBlockID: 1,
		TotalAmount:     "1128750000000000000000",
	}

	//founder
	ah := AssignRules{
		StartBlockID:    1,
		EndBlockID:      INT64_MAX,
		IntervalBlockID: 1,
		TotalAmount:     "5250000000000000000",
	}

	ret := make(map[int64]AssignRules, 10)
	ret[1] = apri
	ret[2] = apub
	ret[3] = ac
