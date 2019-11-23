/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

	"github.com/IBAX-io/go-ibax/packages/model"

	log "github.com/sirupsen/logrus"
)

//Scheduling task to run
func VDESrcTaskStatus(ctx context.Context, d *daemon) error {
	var (
		err error
	)

	m := &model.VDESrcTask{}
	SrcTask, err := m.GetOneTimeTasks() //Query one-time scheduled tasks and generate scheduling requests。
	if err != nil {
		time.Sleep(time.Millisecond * 2)
		log.WithFields(log.Fields{"error": err}).Error("getting GetScheTimeTasks data")
		return err
	}
	if len(SrcTask) > 0 {
		log.Info("Src task found")
		// deal with task data
		for _, item := range SrcTask {
			//fmt.Println("SrcTask:", item.TaskUUID)
			TaskStatus := &model.VDESrcTaskStatus{}
			TaskStatus.TaskUUID = item.TaskUUID
			TaskStatus.ContractRunHttp = item.ContractRunHttp
			TaskStatus.ContractRunEcosystem = item.ContractRunEcosystem
			TaskStatus.ContractRunParms = item.ContractRunParms
			TaskStatus.ContractSrcName = item.ContractSrcName
			TaskStatus.CreateTime = time.Now().Unix()
			err = TaskStatus.Create()
			if err != nil {
				fmt.Println("Create VDESrcTaskStatus table err: ", err)
				log.WithFields(log.Fields{"error": err}).Error("Create VDESrcTaskStatus table!")
				time.Sleep(time.Millisecond * 2)
				continue
			}
			item.TaskRunState = 3
			item.UpdateTime = time.Now().Unix()
			err = item.Updates()
			if err != nil {
				fmt.Println("Update VDESrcTask table err: ", err)
				log.WithFields(log.Fields{"error": err}).Error("Update VDESrcTask table!")
				time.Sleep(time.Millisecond * 2)
				continue
			}
		} //for
	}

	SrcTask, err = m.GetScheTimeTasks() //Query one-time scheduled tasks and generate scheduling requests。
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("getting GetScheTimeTasks data")
		time.Sleep(time.Millisecond * 2)
		return err
	}
	if len(SrcTask) > 0 {
		log.Info("Src task  found")
		// deal with task data
		for _, item := range SrcTask {
			//fmt.Println("SrcTask:", item.TaskUUID)
			TaskStatus := &model.VDESrcTaskStatus{}
			TaskStatus.TaskUUID = item.TaskUUID
			TaskStatus.ContractRunHttp = item.ContractRunHttp
			TaskStatus.ContractRunEcosystem = item.ContractRunEcosystem
			TaskStatus.ContractRunParms = item.ContractRunParms
			TaskStatus.ContractSrcName = item.ContractSrcName
			TaskStatus.CreateTime = time.Now().Unix()
			err = TaskStatus.Create()
			if err != nil {
				fmt.Println("Create VDESrcTaskStatus table err: ", err)
				log.WithFields(log.Fields{"error": err}).Error("Create VDESrcTaskStatus table!")
				time.Sleep(time.Millisecond * 2)
				continue
			}
			item.TaskRunState = 3
			item.UpdateTime = time.Now().Unix()
			err = item.Updates()
			if err != nil {
				fmt.Println("Update VDESrcTask table err: ", err)
				log.WithFields(log.Fields{"error": err}).Error("Update VDESrcTask table!")
				time.Sleep(time.Millisecond * 2)
				continue
			}
		} //for
		time.Sleep(time.Second * 10)
	}

	return nil
}
