/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package block

import (
	"testing"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/IBAX-io/go-ibax/packages/utils"
)

func TestCheckConsensusMode(t *testing.T) {
	for _, c := range []struct {
		mode           int32
		honorNodesOnly bool
		refused        bool
	}{
		{consts.HonorNodeMode, true, false},
		{consts.CandidateNodeMode, true, true},
		{0, true, true},
		{consts.HonorNodeMode, false, false},
		{consts.CandidateNodeMode, false, false},
	} {
		err := checkConsensusMode(&types.BlockHeader{BlockId: 7, ConsensusMode: c.mode}, c.honorNodesOnly)
		if (err != nil) != c.refused {
			t.Errorf("mode %d, honor nodes only %v: error %v, want refused %v", c.mode, c.honorNodesOnly, err, c.refused)
		}
		if err != nil && !utils.IsBanError(err) {
			t.Errorf("mode %d: the refusal %v bans no sender", c.mode, err)
		}
	}
}
