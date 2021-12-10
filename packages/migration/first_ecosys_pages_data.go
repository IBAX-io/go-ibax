/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

var firstEcosystemPagesDataSQL = `INSERT INTO "1_pages" (id, name, value, menu, conditions, app_id, ecosystem) VALUES
	(next_id('1_pages'), 'notifications', '', 'default_menu', 'ContractConditions("@1DeveloperCondition")', '1', '1'),
	(next_id('1_pages'), 'import_app', 'Div(content-wrapper){
    DBFind(@1buffer_data).Columns("id,value->name,value->data").Where({"key": import, "account": #account_id#, "ecosystem": #ecosystem_id#}).Vars(import)
    DBFind(@1buffer_data).Columns("value->app_name,value->pages,value->pages_count,value->blocks,value->blocks_count,value->menu,value->menu_count,value->parameters,value->parameters_count,value->languages,value->languages_count,value->contracts,value->contracts_count,value->tables,value->tables_count").Where({"key": import_info, "account": #account_id#, "ecosystem": #ecosystem_id#}).Vars(info)

    SetTitle("Import - #info_value_app_name#")
    Data(data_info, "DataName,DataCount,DataInfo"){
        Pages,"#info_value_pages_count#","#info_value_pages#"
        Blocks,"#info_value_blocks_count#","#info_value_blocks#"
        Menu,"#info_value_menu_count#","#info_value_menu#"
        Parameters,"#info_value_parameters_count#","#info_value_parameters#"
        Language resources,"#info_value_languages_count#","#info_value_languages#"
        Contracts,"#info_value_contracts_count#","#info_value_contracts#"
        Tables,"#info_value_tables_count#","#info_value_tables#"
    }
    Div(breadcrumb){
        Span(Class: text-muted, Body: "Your data that you can import")
    }

    Div(panel panel-primary){
        ForList(data_info){
            Div(list-group-item){
                Div(row){
                    Div(col-md-10 mc-sm text-left){
                        Span(Class: text-bold, Body: "#DataName#")
                    }
                    Div(col-md-2 mc-sm text-right){
                        If(#DataCount# > 0){
                            Span(Class: text-bold, Body: "(#DataCount#)")
                        }.Else{
                            Span(Class: text-muted, Body: "(0)")
                        }
                    }
                }
                Div(row){
                    Div(col-md-12 mc-sm text-left){
                        If(#DataCount# > 0){
                            Span(Class: h6, Body: "#DataInfo#")
                        }.Else{
                            Span(Class: text-muted h6, Body: "Nothing selected")
                        }
                    }
                }
            }
        }
        If(#import_id# > 0){
            Div(list-group-item text-right){
                VarAsIs(imp_data, "#import_value_data#")
                Button(Body: "Import", Class: btn btn-primary, Page: @1apps_list).CompositeContract(@1Import, "#imp_data#")
            }
        }
    }
}', 'developer_menu', 'ContractConditions("@1DeveloperCondition")', '1', '1'),
	(next_id('1_pages'), 'import_upload', 'Div(content-wrapper){
        SetTitle("Import")
        Div(breadcrumb){
            Span(Class: text-muted, Body: "Select payload that you want to import")
        }
        Form(panel panel-primary){
            Div(list-group-item){
                Input(Name: Data, Type: file)
            }
            Div(list-group-item text-right){
                Button(Body: "Load", Class: btn btn-primary, Contract: @1ImportUpload, Page: @1import_app)
            }
        }
    }', 'developer_menu', 'ContractConditions("@1DeveloperCondition")', '1', '1');
`
