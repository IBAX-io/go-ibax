package syspar

import (
	"testing"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/types"
)

func setFirstBlock(t *testing.T, data *types.FirstBlock) {
	t.Helper()
	mutex.Lock()
	before, model := firstBlockData, runModel
	firstBlockData = data
	mutex.Unlock()
	t.Cleanup(func() {
		mutex.Lock()
		firstBlockData, runModel = before, model
		mutex.Unlock()
	})
}

// TestHonorNodesOnlyNeverCandidate: the first block of a chain of honor nodes only, as of any
// chain of the BFT consensus, keeps it out of the candidate node mode whatever sets the mode, and
// its candidates are none without a read of the table
func TestHonorNodesOnlyNeverCandidate(t *testing.T) {
	setFirstBlock(t, &types.FirstBlock{HonorNodesOnly: 1, BftConsensus: 1})
	SetRunModel(consts.CandidateNodeMode)
	if IsCandidateNodeMode() || !IsHonorNodeMode() {
		t.Fatalf("candidate %v, honor %v", IsCandidateNodeMode(), IsHonorNodeMode())
	}
	// the database is not open: a read of the table would fail
	if nodes, err := GetCandidateNodes(); err != nil || len(nodes) != 0 {
		t.Fatalf("candidates %v, %v", nodes, err)
	}

	setFirstBlock(t, &types.FirstBlock{})
	SetRunModel(consts.CandidateNodeMode)
	if !IsCandidateNodeMode() || IsHonorNodeMode() {
		t.Fatalf("without the flag: candidate %v, honor %v", IsCandidateNodeMode(), IsHonorNodeMode())
	}
	SetRunModel(consts.HonorNodeMode)
	if IsCandidateNodeMode() || !IsHonorNodeMode() {
		t.Fatalf("without the flag, honor mode: candidate %v, honor %v", IsCandidateNodeMode(), IsHonorNodeMode())
	}
}
