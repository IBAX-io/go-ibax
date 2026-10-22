/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package notificator

import (
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	log "github.com/sirupsen/logrus"
)

type notificationRecord struct {
	EcosystemID  string `json:"ecosystem"`
	RoleID       string `json:"role_id"`
	RecordsCount int64  `json:"count"`
}

func getEcosystemNotificationStats(ecosystemID int64, users []string) (map[string]*[]notificationRecord, error) {
	result, err := sqldb.GetNotificationsCount(ecosystemID, users)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting notification count")
		return nil, err
	}

	return parseRecipientNotification(result, ecosystemID), nil
}

func parseRecipientNotification(rows []sqldb.NotificationsCount, systemID int64) map[string]*[]notificationRecord {
	recipientNotifications := make(map[string]*[]notificationRecord)

	for _, r := range rows {
		if r.RecipientID == 0 {
			continue
		}

		roleNotifications := notificationRecord{
			EcosystemID:  converter.Int64ToStr(systemID),
			RoleID:       converter.Int64ToStr(r.RoleID),
			RecordsCount: r.Count,
		}

		nr, ok := recipientNotifications[r.Account]
		if ok {
			*nr = append(*nr, roleNotifications)
			continue
		}

		records := []notificationRecord{
			roleNotifications,
		}

		recipientNotifications[r.Account] = &records
	}

	return recipientNotifications
}
