}

func (bc *blocksCounter) count(state blockGenerationState) (int, error) {
	blockchain := &model.Block{}
	blocks, err := blockchain.GetNodeBlocksAtTime(state.start, state.start.Add(state.duration), state.nodePosition)
	if err != nil {
		return 0, err
	}
	return len(blocks), nil
}
