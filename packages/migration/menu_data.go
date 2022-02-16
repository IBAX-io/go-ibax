/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

var menuDataSQL = `INSERT INTO "1_menu" (id, name, value, conditions, ecosystem) VALUES
(next_id('1_menu'), 'developer_menu', 'MenuItem(Title:"Import", Page:@1import_upload, Icon:"icon-cloud-upload")', 'ContractAccess("@1EditMenu")','{{.Ecosystem}}');
`
