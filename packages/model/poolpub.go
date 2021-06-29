package model

var (
	MintCountCh = make(chan *MinterCount, 10)
)

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
