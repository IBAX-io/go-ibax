/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package daemons

import (
	"context"

	log "github.com/sirupsen/logrus"

	"time"

	"github.com/IBAX-io/go-ibax/packages/model"
)

func VDEDestDataStatus(ctx context.Context, d *daemon) error {
	var (
		err error
	)

		return nil
	}

	// deal with task data
	for _, item := range ShareData {
		//fmt.Println("TaskUUID,DataUUID:", item.TaskUUID, item.DataUUID)
		m := &model.VDEDestDataHash{}
		dataHash, err := m.GetOneByTaskUUIDAndDataUUID(item.TaskUUID, item.DataUUID)
		if err != nil {
			//log.WithFields(log.Fields{"error": err}).Error("getting one hash data by TaskUUID ans DataUUID")
			//time.Sleep(time.Second * 1)
			continue
		}
		if item.Hash == dataHash.Hash {
			item.HashState = 1 //
			//fmt.Println("Hash match!")
		} else {
			item.HashState = 2 //
			log.WithFields(log.Fields{"error": err}).Error("Hash does not match！")
		}
		err = item.Updates()
		if err != nil {
			log.WithError(err)
			continue
		}

	} //for
	return nil
}
