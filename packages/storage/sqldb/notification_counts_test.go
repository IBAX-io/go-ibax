/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package sqldb

import (
	"fmt"
	"sort"
	"testing"

	"github.com/IBAX-io/go-ibax/packages/migration"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	countAccountA = "0000-0000-0000-0000-0001"
	countAccountB = "0000-0000-0000-0000-0002"
	countAccountC = "0000-0000-0000-0000-0003"
)

// recountNotifications counts the open notifications of the accounts row by row, as the counts
// of notification_counts must give them: for the account, and for every role it takes part in,
// in the ecosystem, those to the role that no account or the account processes
func recountNotifications(t *testing.T, tx *gorm.DB, ecosystem int64, accounts []string) []string {
	var rows []NotificationsCount
	require.NoError(t, tx.Raw(`SELECT k.id AS recipient_id, k.account, '0' AS role_id,
			(SELECT count(*) FROM "1_notifications" n WHERE n.ecosystem = k.ecosystem AND n.closed = 0
				AND n.notification->>'type' = '1' AND n.recipient->>'account' = k.account) AS count
		FROM "1_keys" k WHERE k.ecosystem = ? AND k.account IN (?)
		UNION ALL
		SELECT k.id, k.account, r.role_id,
			(SELECT count(*) FROM "1_notifications" n WHERE n.ecosystem = k.ecosystem AND n.closed = 0
				AND n.notification->>'type' = '2' AND n.recipient->>'role_id' = r.role_id
				AND (n.date_start_processing = 0 OR n.processing_info->>'account' = k.account))
		FROM "1_keys" k
		JOIN (SELECT DISTINCT member->>'account' AS account, role->>'id' AS role_id FROM "1_roles_participants"
			WHERE ecosystem = ? AND deleted = 0) r ON r.account = k.account
		WHERE k.ecosystem = ? AND k.account IN (?)`,
		ecosystem, accounts, ecosystem, ecosystem, accounts).Scan(&rows).Error)
	return countLines(rows)
}

func countLines(rows []NotificationsCount) []string {
	lines := make([]string, 0, len(rows))
	for _, r := range rows {
		lines = append(lines, fmt.Sprintf("%d %s role %d: %d", r.RecipientID, r.Account, r.RoleID, r.Count))
	}
	sort.Strings(lines)
	return lines
}

// TestNotificationCounts runs on the database of a node, named by IBAX_TEST_DSN, in the transaction
// testDB rolls back: it creates notification_counts and its trigger, as the migrations of a new
// chain do, and checks after each change of 1_notifications that GetNotificationsCount gives the
// counts of the rows themselves, and that the trigger keeps exactly the keys of the open
// notifications at their counts, as the state hash needs
func TestNotificationCounts(t *testing.T) {
	tx := testDB(t)
	require.NoError(t, tx.Exec(migration.NotificationCountsSQL).Error)
	for _, q := range []string{`DELETE FROM "1_notifications"`, `DELETE FROM "1_roles_participants"`, `DELETE FROM notification_counts`} {
		require.NoError(t, tx.Exec(q).Error)
	}
	for i, account := range []string{countAccountA, countAccountB, countAccountC} {
		require.NoError(t, tx.Exec(`INSERT INTO "1_keys" (id, ecosystem, account) VALUES (?, 1, ?), (?, 2, ?)`,
			-1-i, account, -1-i, account).Error)
	}
	member := func(id int64, ecosystem int64, role int64, account string, deleted int64) {
		require.NoError(t, tx.Exec(`INSERT INTO "1_roles_participants" (id, ecosystem, role, member, deleted) VALUES (?, ?, ?, ?, ?)`,
			id, ecosystem, fmt.Sprintf(`{"id": "%d", "name": "r%d"}`, role, role), fmt.Sprintf(`{"account": "%s"}`, account), deleted).Error)
	}
	member(1, 1, 7, countAccountA, 0)
	member(2, 1, 7, countAccountB, 0)
	member(3, 1, 7, countAccountB, 0) // twice in the role: counted once
	member(4, 1, 8, countAccountA, 1) // left the role
	member(5, 2, 9, countAccountA, 0) // a role of another ecosystem
	member(6, 1, 9, countAccountC, 0)

	nextID := int64(0)
	notify := func(ecosystem int64, kind int, recipient string, started int64, processor string, closed int64) int64 {
		nextID++
		processing := "{}"
		if processor != "" {
			processing = fmt.Sprintf(`{"account": "%s"}`, processor)
		}
		require.NoError(t, tx.Exec(`INSERT INTO "1_notifications" (id, ecosystem, recipient, notification, processing_info, date_start_processing, closed)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, nextID, ecosystem, recipient, fmt.Sprintf(`{"type": %d}`, kind), processing, started, closed).Error)
		return nextID
	}
	toAccount := func(a string) string { return fmt.Sprintf(`{"account": "%s"}`, a) }
	toRole := func(r int64) string { return fmt.Sprintf(`{"role_id": "%d"}`, r) }

	a1 := notify(1, 1, toAccount(countAccountA), 0, "", 0)
	notify(1, 1, toAccount(countAccountA), 0, "", 0)
	notify(1, 1, toAccount(countAccountA), 0, "", 1)    // closed
	notify(2, 1, toAccount(countAccountA), 0, "", 0)    // another ecosystem
	r1 := notify(1, 2, toRole(7), 0, "", 0)             // nobody processes it
	notify(1, 2, toRole(7), 10, countAccountA, 0)       // A processes it
	r3 := notify(1, 2, toRole(7), 10, countAccountB, 0) // B processes it
	notify(1, 2, toRole(7), 0, "", 1)                   // closed
	notify(1, 2, toRole(8), 0, "", 0)                   // a role A left
	notify(1, 2, toRole(9), 0, "", 0)                   // C's role
	notify(1, 3, toAccount(countAccountA), 0, "", 0)    // no such type
	notify(1, 1, `{"member_id": "1"}`, 0, "", 0)        // no account
	notify(1, 2, `{}`, 0, "", 0)                        // no role

	accounts := []string{countAccountA, countAccountB, countAccountC}
	check := func(step string, want []string) {
		got, err := GetNotificationsCount(1, accounts)
		require.NoError(t, err, step)
		require.Equal(t, want, countLines(got), step)
		require.Equal(t, want, recountNotifications(t, tx, 1, accounts), step+": the rows themselves")
		var stale int64
		require.NoError(t, tx.Raw(`SELECT count(*) FROM (
				SELECT ecosystem, kind, role_id, account, count FROM notification_counts
				EXCEPT ALL
				SELECT n.ecosystem, k.kind, k.role_id, k.account, count(*) FROM "1_notifications" n, notification_count_key(n) k
				WHERE k.kind IS NOT NULL GROUP BY 1, 2, 3, 4) d`).Scan(&stale).Error, step)
		require.Zero(t, stale, step+": notification_counts has a count the rows do not give")
		require.NoError(t, tx.Raw(`SELECT count(*) FROM (
				SELECT n.ecosystem, k.kind, k.role_id, k.account, count(*) FROM "1_notifications" n, notification_count_key(n) k
				WHERE k.kind IS NOT NULL GROUP BY 1, 2, 3, 4
				EXCEPT ALL
				SELECT ecosystem, kind, role_id, account, count FROM notification_counts) d`).Scan(&stale).Error, step)
		require.Zero(t, stale, step+": the rows give a count notification_counts has not")
	}
	check("inserted", []string{
		"-1 0000-0000-0000-0000-0001 role 0: 2",
		"-1 0000-0000-0000-0000-0001 role 7: 2",
		"-2 0000-0000-0000-0000-0002 role 0: 0",
		"-2 0000-0000-0000-0000-0002 role 7: 2",
		"-3 0000-0000-0000-0000-0003 role 0: 0",
		"-3 0000-0000-0000-0000-0003 role 9: 1",
	})

	exec := func(q string, args ...any) { require.NoError(t, tx.Exec(q, args...).Error) }
	exec(`UPDATE "1_notifications" SET closed = 1 WHERE id = ?`, r1)
	exec(`UPDATE "1_notifications" SET processing_info = ? WHERE id = ?`, toAccount(countAccountA), r3)
	exec(`UPDATE "1_notifications" SET page_name = 'p' WHERE ecosystem = 1`) // no key changes
	exec(`DELETE FROM "1_notifications" WHERE id = ?`, a1)
	check("closed, handed to A, deleted", []string{
		"-1 0000-0000-0000-0000-0001 role 0: 1",
		"-1 0000-0000-0000-0000-0001 role 7: 2",
		"-2 0000-0000-0000-0000-0002 role 0: 0",
		"-2 0000-0000-0000-0000-0002 role 7: 0",
		"-3 0000-0000-0000-0000-0003 role 0: 0",
		"-3 0000-0000-0000-0000-0003 role 9: 1",
	})

	// a closed notification opened again, one to the role started by nobody, one moved to ecosystem 2
	exec(`UPDATE "1_notifications" SET closed = 0 WHERE id = ?`, r1)
	exec(`UPDATE "1_notifications" SET date_start_processing = 0 WHERE id = ?`, r3)
	exec(`UPDATE "1_notifications" SET ecosystem = 2 WHERE ecosystem = 1 AND recipient->>'role_id' = '9'`)
	check("reopened, unstarted, moved", []string{
		"-1 0000-0000-0000-0000-0001 role 0: 1",
		"-1 0000-0000-0000-0000-0001 role 7: 3",
		"-2 0000-0000-0000-0000-0002 role 0: 0",
		"-2 0000-0000-0000-0000-0002 role 7: 2",
		"-3 0000-0000-0000-0000-0003 role 0: 0",
		"-3 0000-0000-0000-0000-0003 role 9: 0",
	})

	// a rolled back statement leaves the counts as they were
	require.NoError(t, tx.SavePoint("rollback").Error)
	exec(`DELETE FROM "1_notifications"`)
	require.NoError(t, tx.RollbackTo("rollback").Error)
	check("rolled back", []string{
		"-1 0000-0000-0000-0000-0001 role 0: 1",
		"-1 0000-0000-0000-0000-0001 role 7: 3",
		"-2 0000-0000-0000-0000-0002 role 0: 0",
		"-2 0000-0000-0000-0000-0002 role 7: 2",
		"-3 0000-0000-0000-0000-0003 role 0: 0",
		"-3 0000-0000-0000-0000-0003 role 9: 0",
	})
	exec(`DELETE FROM "1_notifications"`)
	var open int64
	require.NoError(t, tx.Raw(`SELECT coalesce(sum(count), 0) FROM notification_counts`).Scan(&open).Error)
	require.Zero(t, open, "all deleted")
	var keys int64
	require.NoError(t, tx.Raw(`SELECT count(*) FROM notification_counts`).Scan(&keys).Error)
	require.Zero(t, keys, "a key of count 0 is deleted")

	got, err := GetNotificationsCount(1, nil)
	require.NoError(t, err)
	require.Empty(t, got)
}
