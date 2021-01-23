/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package obs

var menuDataSQL = `INSERT INTO "1_menu" (id, name, value, conditions, ecosystem) VALUES
(next_id('1_menu'), 'admin_menu', 'MenuItem(Title:"Application", Page:apps_list, Icon:"icon-folder")
MenuItem(Title:"Ecosystem parameters", Page:params_list, Icon:"icon-settings")
	MenuItem(Title:"Language resources", Page:app_langres, Icon:"icon-globe")
	MenuItem(Title:"Binary data", Page:app_binary, Icon:"icon-layers")
}
MenuItem(Title:"Dashboard", Page:admin_dashboard, Icon:"icon-wrench")', 'ContractConditions("MainCondition")', '%[1]d');
`
