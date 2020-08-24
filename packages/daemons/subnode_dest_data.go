/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons
	"github.com/IBAX-io/go-ibax/packages/model"

	log "github.com/sirupsen/logrus"
)

func SubNodeDestData(ctx context.Context, d *daemon) error {
	var (
		err error
	)

	m := &model.SubNodeDestData{}
	ShareData, err := m.GetAllByDataStatus(0) //0not deal，1deal ok，2 fail
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("getting all untreated task data")
		time.Sleep(time.Millisecond * 2)
		return err
	}
	if len(ShareData) == 0 {
		//log.Info("task data not found")
		time.Sleep(time.Millisecond * 2)
		return nil
	}

	// deal with task data
	for _, item := range ShareData {
		//fmt.Println("TaskUUID,DataUUID:", item.TaskUUID, item.DataUUID)
		if item.TranMode == 1 { //1 hash uptochain
			DestDataHash := model.SubNodeDestDataHash{
				DataUUID:           item.DataUUID,
				TaskUUID:           item.TaskUUID,
				Hash:               item.Hash,
				Data:               item.Data,
				DataInfo:           item.DataInfo,
				SubNodeSrcPubkey:   item.SubNodeSrcPubkey,
				SubNodeDestPubkey:  item.SubNodeDestPubkey,
				SubNodeDestIP:      item.SubNodeDestIP,
				SubNodeAgentPubkey: item.SubNodeAgentPubkey,
				SubNodeAgentIP:     item.SubNodeAgentIP,
				AgentMode:          item.AgentMode,
				TranMode:           item.TranMode,
				AuthState:          1,
				SignState:          1,
				HashState:          1,
				CreateTime:         item.CreateTime}
			//CreateTime: time.Now().Unix()}

			if err = DestDataHash.Create(); err != nil {
				log.WithFields(log.Fields{"error": err}).Error("Insert subnode_dest_data_hash table failed")
				continue
			}
			fmt.Println("Insert subnode_dest_data_hash table ok, DataUUID:", item.DataUUID)
		} else if item.TranMode == 3 { // data under chain
			DestDataStatus := model.SubNodeDestDataStatus{
				DataUUID:           item.DataUUID,
				TaskUUID:           item.TaskUUID,
				Hash:               item.Hash,
				Data:               item.Data,
				DataInfo:           item.DataInfo,
				SubNodeSrcPubkey:   item.SubNodeSrcPubkey,
				SubNodeDestPubkey:  item.SubNodeDestPubkey,
				SubNodeDestIP:      item.SubNodeDestIP,
				SubNodeAgentPubkey: item.SubNodeAgentPubkey,
				SubNodeAgentIP:     item.SubNodeAgentIP,
				AgentMode:          item.AgentMode,
				TranMode:           item.TranMode,
				AuthState:          1,
				SignState:          1,
				HashState:          1,
				CreateTime:         item.CreateTime}
			//CreateTime: time.Now().Unix()}

			if err = DestDataStatus.Create(); err != nil {
				log.WithFields(log.Fields{"error": err}).Error("Insert subnode_dest_data_status table failed")
				continue
			}
			fmt.Println("Insert subnode_dest_data_status table ok, DataUUID:", item.DataUUID)
		} else {
			log.WithFields(log.Fields{"error": err}).Error("TranMode err!")
			item.DataState = 3
			err = item.Updates()
			//err = item.Delete()
			if err != nil {
				log.WithError(err)
				continue
			}
			continue
		}
		item.DataState = 1 // success
		err = item.Updates()
		//err = item.Delete()
		if err != nil {
			log.WithError(err)
			continue
		}
	} //for
	return nil
}
