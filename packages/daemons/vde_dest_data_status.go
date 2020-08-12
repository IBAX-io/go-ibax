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

	m := &model.VDEDestDataStatus{}
	ShareData, err := m.GetAllByHashState(0) //0not to deal，1deal ok，2fail,3
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("getting all untreated dest data status")
		time.Sleep(time.Millisecond * 2)
		return err
	}
	if len(ShareData) == 0 {
		//log.Info("dest task data status not found")
		//fmt.Println("dest task data status not found")
		time.Sleep(time.Millisecond * 2)
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

