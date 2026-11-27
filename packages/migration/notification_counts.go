/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

// NotificationCountsSQL creates notification_counts, the counts of the open notifications that
// clients are pushed, and the trigger that keeps them with 1_notifications. A key is a kind, the
// recipient and the account:
//
//	kind 1, the notifications of an account:          role "0", the account of the recipient
//	kind 2, of a role, which no account processes:   the role of the recipient, account ""
//	kind 2, of a role, which an account processes:   the role of the recipient, the account
//
// The count of an account is that of its kind 1 key, and for every role of the account those of
// the kind 2 keys of the role with account "" and with the account. Reading it costs a few index
// lookups whatever the number of open notifications, where counting them cost a row each. The
// trigger counts what reaches the table: the rows a block writes at its end, the statements of a
// block and those of a rollback; a snapshot copies the counts with the notifications and creates
// the trigger after both. The table is of the state hash, so it holds exactly the counts the open
// notifications give, whatever path wrote them: a key whose count comes to 0 is deleted. No
// contract reads it.
const NotificationCountsSQL = `
DROP TABLE IF EXISTS "notification_counts";
CREATE TABLE "notification_counts" (
	"ecosystem" bigint NOT NULL,
	"kind" smallint NOT NULL,
	"role_id" text NOT NULL,
	"account" text NOT NULL,
	"count" bigint NOT NULL,
	PRIMARY KEY ("ecosystem", "kind", "role_id", "account")
);

CREATE OR REPLACE FUNCTION notification_count_key(n "1_notifications", OUT kind smallint, OUT role_id text, OUT account text) AS
$$
BEGIN
	IF n.closed <> 0 THEN
		RETURN;
	END IF;
	CASE n.notification->>'type'
	WHEN '1' THEN
		kind := 1; role_id := '0'; account := n.recipient->>'account';
	WHEN '2' THEN
		kind := 2; role_id := n.recipient->>'role_id';
		IF n.date_start_processing = 0 THEN
			account := '';
		ELSE
			account := n.processing_info->>'account';
		END IF;
	ELSE
		RETURN;
	END CASE;
	IF role_id IS NULL OR account IS NULL THEN
		kind := NULL;
	END IF;
END
$$ LANGUAGE plpgsql IMMUTABLE;

CREATE OR REPLACE FUNCTION notification_counts_keep() RETURNS trigger AS
$$
DECLARE
	ok smallint; orole text; oaccount text;
	nk smallint; nrole text; naccount text;
BEGIN
	IF TG_OP <> 'INSERT' THEN
		SELECT * INTO ok, orole, oaccount FROM notification_count_key(OLD);
	END IF;
	IF TG_OP <> 'DELETE' THEN
		SELECT * INTO nk, nrole, naccount FROM notification_count_key(NEW);
	END IF;
	IF TG_OP = 'UPDATE' AND OLD.ecosystem = NEW.ecosystem
		AND (ok, orole, oaccount) IS NOT DISTINCT FROM (nk, nrole, naccount) THEN
		RETURN NULL;
	END IF;
	IF ok IS NOT NULL THEN
		DELETE FROM "notification_counts"
		WHERE "ecosystem" = OLD.ecosystem AND "kind" = ok AND "role_id" = orole AND "account" = oaccount AND "count" = 1;
		IF NOT FOUND THEN
			UPDATE "notification_counts" SET "count" = "count" - 1
			WHERE "ecosystem" = OLD.ecosystem AND "kind" = ok AND "role_id" = orole AND "account" = oaccount;
		END IF;
	END IF;
	IF nk IS NOT NULL THEN
		INSERT INTO "notification_counts" AS c VALUES (NEW.ecosystem, nk, nrole, naccount, 1)
		ON CONFLICT ("ecosystem", "kind", "role_id", "account") DO UPDATE SET "count" = c."count" + 1;
	END IF;
	RETURN NULL;
END
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS "notification_counts_keep" ON "1_notifications";
CREATE TRIGGER "notification_counts_keep" AFTER INSERT OR UPDATE OR DELETE ON "1_notifications"
	FOR EACH ROW EXECUTE FUNCTION notification_counts_keep();
`
