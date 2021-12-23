/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package clb

var menuDataSQL = `INSERT INTO "1_menu" (id, name, value, conditions, ecosystem) VALUES
(next_id('1_menu'), 'admin_menu', 'MenuItem(Title:"Application", Page:apps_list, Icon:"icon-folder")
MenuItem(Title:"Ecosystem parameters", Page:params_list, Icon:"icon-settings")
MenuItem(Title:"Menu", Page:menus_list, Icon:"icon-list")
MenuItem(Title:"Confirmations", Page:confirmations, Icon:"icon-check")
MenuItem(Title:"Import", Page:import_upload, Icon:"icon-cloud-upload")
MenuItem(Title:"Export", Page:export_resources, Icon:"icon-cloud-download")
MenuGroup(Title:"Resources", Icon:"icon-share"){
	MenuItem(Title:"Pages", Page:app_pages, Icon:"icon-screen-desktop")
	MenuItem(Title:"Blocks", Page:app_blocks, Icon:"icon-grid")
	MenuItem(Title:"Tables", Page:app_tables, Icon:"icon-docs")
	MenuItem(Title:"Contracts", Page:app_contracts, Icon:"icon-briefcase")
	MenuItem(Title:"Application parameters", Page:app_params, Icon:"icon-wrench")
	MenuItem(Title:"Language resources", Page:app_langres, Icon:"icon-globe")
	MenuItem(Title:"Binary data", Page:app_binary, Icon:"icon-layers")
}
MenuItem(Title:"Dashboard", Page:admin_dashboard, Icon:"icon-wrench")', 'ContractConditions("MainCondition")', '%[1]d');
`
