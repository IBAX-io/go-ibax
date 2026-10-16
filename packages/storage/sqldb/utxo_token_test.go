package sqldb

import "testing"

func TestUpdateTxInputsOrder(t *testing.T) {
	keys := []KeyUTXO{{Ecosystem: 2, KeyId: 5}, {Ecosystem: 1, KeyId: 9}, {Ecosystem: 1, KeyId: 3}, {Ecosystem: 3, KeyId: 1}}
	for run := 0; run < 50; run++ {
		outputs := make(map[KeyUTXO][]SpentInfo)
		inputs := make(map[KeyUTXO][]SpentInfo)
		for _, k := range keys {
			outputs[k] = []SpentInfo{{OutputKeyId: k.KeyId, Ecosystem: k.Ecosystem}, {OutputKeyId: k.KeyId, Ecosystem: k.Ecosystem}}
			inputs[k] = nil
		}
		UpdateTxInputs([]byte{1}, inputs, outputs)
		want := map[KeyUTXO]int32{keys[2]: 0, keys[1]: 2, keys[0]: 4, keys[3]: 6}
		for k, first := range want {
			if outputs[k][0].InputIndex != first || outputs[k][1].InputIndex != first+1 {
				t.Fatalf("run %d key %+v: input_index %d, %d, want %d, %d", run, k,
					outputs[k][0].InputIndex, outputs[k][1].InputIndex, first, first+1)
			}
		}
	}
}
