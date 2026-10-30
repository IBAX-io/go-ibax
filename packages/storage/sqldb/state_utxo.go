/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

// The outputs of the accounts of a block are in memory, in the map the block reads at its start.
// The state keeps it: a view reads the unused outputs of an account under the lock of the outputs,
// and a commit that spends or adds outputs of an account fails the views that read them before.

// utxoRead is a read of the outputs of an account at the commit seq
type utxoRead struct {
	key KeyUTXO
	seq uint64
}

// SetOutputs makes the state keep the outputs of the block
func (st *BlockState) SetOutputs(outputs map[KeyUTXO][]SpentInfo) {
	st.mu.Lock()
	st.utxoMu.Lock()
	st.outputs = outputs
	st.utxoMu.Unlock()
	st.utxoSeq = make(map[KeyUTXO]uint64)
	st.mu.Unlock()
}

// AddOutputs puts the outputs of accounts the state did not read at its start before those the
// block added to them, as if it had read them at its start
func (st *BlockState) AddOutputs(outputs []SpentInfo) {
	if len(outputs) == 0 {
		return
	}
	read := make(map[KeyUTXO][]SpentInfo)
	PutAllOutputsMap(outputs, read)
	st.mu.Lock()
	defer st.mu.Unlock()
	st.utxoMu.Lock()
	for key, list := range read {
		st.outputs[key] = append(list, st.outputs[key]...)
	}
	st.utxoMu.Unlock()
	st.seq++
	for key := range read {
		st.utxoSeq[key] = st.seq
	}
	st.pub.Store(st.seq)
}

// UnusedOutputs are the outputs of the account the block has not spent
func (v *TxView) UnusedOutputs(key KeyUTXO) []SpentInfo {
	st := v.st
	st.utxoMu.RLock()
	seq := st.pub.Load()
	list := GetUnusedOutputsMap(key, st.outputs)
	st.utxoMu.RUnlock()
	if !v.ordered {
		v.utxo = append(v.utxo, utxoRead{key: key, seq: seq})
	}
	return list
}

// ApplyOutputs spends the inputs of a transaction the block committed and adds its outputs
func (st *BlockState) ApplyOutputs(hash []byte, blockID int64, inputs, outputs map[KeyUTXO][]SpentInfo) {
	if len(inputs) == 0 && len(outputs) == 0 {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	st.utxoMu.Lock()
	UpdateTxInputs(hash, blockID, inputs, st.outputs)
	InsertTxOutputs(hash, outputs, st.outputs)
	st.utxoMu.Unlock()
	st.seq++
	for key := range inputs {
		st.utxoSeq[key] = st.seq
	}
	for key := range outputs {
		st.utxoSeq[key] = st.seq
	}
	st.pub.Store(st.seq)
}

// utxoHolds tells the outputs the view read are as it read them. The caller holds the lock.
func (st *BlockState) utxoHolds(v *TxView) bool {
	for _, r := range v.utxo {
		if st.utxoSeq[r.key] > r.seq {
			return false
		}
	}
	return true
}
