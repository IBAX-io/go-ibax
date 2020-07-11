/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package obs

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
