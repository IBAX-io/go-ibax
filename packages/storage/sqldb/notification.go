/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"fmt"

	"github.com/IBAX-io/go-ibax/packages/converter"
)

const (
	notificationTableSuffix = "_notifications"

	NotificationTypeSingle = 1
	NotificationTypeRole   = 2
)

// Notification structure
type Notification struct {
	ecosystem           int64
	ID                  int64  `gorm:"primary_key;not null"`
	Recipient           string `gorm:"type:jsonb"`
	Sender              string `gorm:"type:jsonb"`
	Notification        string `gorm:"type:jsonb"`
	PageParams          string `gorm:"type:jsonb"`
	ProcessingInfo      string `gorm:"type:jsonb"`
	PageName            string `gorm:"size:255"`
	DateCreated         int64
	DateStartProcessing int64
	DateClosed          int64
	Closed              bool
}

// SetTablePrefix set table Prefix
func (n *Notification) SetTablePrefix(tablePrefix string) {
	n.ecosystem = converter.StrToInt64(tablePrefix)
}

// TableName returns table name
func (n *Notification) TableName() string {
	if n.ecosystem == 0 {
		n.ecosystem = 1
	}
	return `1_notifications`
}

type NotificationsCount struct {
	RecipientID int64  `gorm:"recipient_id"`
	Account     string `gorm:"account"`
	RoleID      int64  `gorm:"role_id"`
	Count       int64  `gorm:"count"`
}

// notificationsCountSQL reads the counts of notification_counts, which a trigger keeps with
// 1_notifications (migration.NotificationCountsSQL): for every account its open notifications of
// type 1, and for every role it takes part in, in the ecosystem, the open notifications of type 2
// to the role that no account processes or that the account processes. It costs a few index
// lookups per account and role, whatever the number of notifications.
const notificationsCountSQL = `SELECT k.id AS recipient_id, k.account, '0' AS role_id, coalesce(c.count, 0) AS count
FROM "1_keys" k
LEFT JOIN notification_counts c ON c.ecosystem = k.ecosystem AND c.kind = 1 AND c.role_id = '0' AND c.account = k.account
WHERE k.ecosystem = ? AND k.account IN (?)
UNION ALL
SELECT k.id, k.account, r.role_id, coalesce(sum(c.count), 0)
FROM "1_keys" k
JOIN (SELECT DISTINCT member->>'account' AS account, role->>'id' AS role_id FROM "1_roles_participants"
	WHERE ecosystem = ? AND deleted = 0 AND member->>'account' IN (?)) r ON r.account = k.account
LEFT JOIN notification_counts c ON c.ecosystem = k.ecosystem AND c.kind = 2 AND c.role_id = r.role_id AND c.account IN ('', k.account)
WHERE k.ecosystem = ? AND k.account IN (?)
GROUP BY k.id, k.account, r.role_id`

// GetNotificationsCount returns the numbers of open notifications of the accounts in the
// ecosystem: one row with role 0 per account, for the notifications to the account, and one row
// per role of the account, for those to the role.
func GetNotificationsCount(ecosystemID int64, accounts []string) ([]NotificationsCount, error) {
	result := make([]NotificationsCount, 0, len(accounts))
	if len(accounts) == 0 {
		return result, nil
	}
	err := GetDB(nil).Raw(notificationsCountSQL, ecosystemID, accounts, ecosystemID, accounts, ecosystemID, accounts).
		Scan(&result).Error
	if err != nil {
		return nil, err
	}
	return result, nil
}

func getNotificationCountFilter(users []int64, ecosystemID int64) (filter string, params []any) {
	filter = fmt.Sprintf(` WHERE closed = 0 and ecosystem = '%d' `, ecosystemID)

	if len(users) > 0 {
		filter += `AND recipient->>'member_id' IN (?) `
		usersStrs := []string{}
		for _, user := range users {
			usersStrs = append(usersStrs, converter.Int64ToStr(user))
		}
		params = append(params, usersStrs)
	}

	return
}
