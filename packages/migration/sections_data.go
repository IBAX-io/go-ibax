/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

var sectionsDataSQL = `
INSERT INTO "1_sections" ("id","title","urlname","page","roles_access", "status", "ecosystem") VALUES
(next_id('1_sections'), 'Home', 'home', 'default_page', '[]', 2, '{{.Ecosystem}}'),
(next_id('1_sections'), 'Developer', 'developer', 'developer_index', '[]', 1, '{{.Ecosystem}}');
`
