package model

var (
	MintCountCh = make(chan *MinterCount, 10)
)

func Put_MintCount() error {

	return nil
}

func Deal_MintCount() error {
	//for {
	//	select {
	//	case dat := <-MintCountCh:
	//		err := dat.Insert_redisdb()
	//		if err != nil {
	//			log.Info("Deal_MintCount Insert_redisdb: ", err.Error())
	//			//time.Sleep(4000)
	//		}
}
