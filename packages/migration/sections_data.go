/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

var sectionsDataSQL = `
INSERT INTO "1_sections" ("id","title","urlname","page","roles_access", "status", "ecosystem") VALUES
