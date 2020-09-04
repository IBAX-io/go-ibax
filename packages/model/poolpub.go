package model

var (
	MintCountCh = make(chan *MinterCount, 10)
)

func Deal_MintCount() error {
	//for {
	//	select {
	//	case dat := <-MintCountCh:
	//		err := dat.Insert_redisdb()
	//		if err != nil {
	//			log.Info("Deal_MintCount Insert_redisdb: ", err.Error())
	//			//time.Sleep(4000)
	//		}
	//		//time.Sleep(5*time.Second)
	//		//MintCountDealCh<-dat
	//	}
	//
	//}
	return nil
}
