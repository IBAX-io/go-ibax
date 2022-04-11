package pbgo

func (m *FirstBlock) TxType() TransactionTypes  { return TransactionTypes_FIRSTBLOCK }
func (m *StopNetwork) TxType() TransactionTypes { return TransactionTypes_STOPNETWORK }
