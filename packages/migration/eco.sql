DROP TABLE IF EXISTS "1_ecosystems";
CREATE TABLE "1_ecosystems" (
"id" bigint NOT NULL DEFAULT '0',
"name" VARCHAR (255) NOT NULL DEFAULT '',
"info" jsonb,
"fee_mode_info" jsonb,
"is_valued" bigint NOT NULL DEFAULT '0',
"emission_amount" jsonb,
"token_symbol" VARCHAR (255),
"token_name" VARCHAR (255),
"type_emission" bigint NOT NULL DEFAULT '0',
"type_withdraw" bigint NOT NULL DEFAULT '0',
"control_mode" bigint NOT NULL DEFAULT '1',
"digits" bigint NOT NULL DEFAULT '0'
);
ALTER TABLE ONLY "1_ecosystems" ADD CONSTRAINT "1_ecosystems_pkey" PRIMARY KEY (id);
DROP TABLE IF EXISTS "1_platform_parameters";
CREATE TABLE "1_platform_parameters" (
"id" bigint NOT NULL DEFAULT '0',
"name" VARCHAR (255) NOT NULL DEFAULT '',
"value" text NOT NULL DEFAULT '',
"conditions" text NOT NULL DEFAULT ''
);
ALTER TABLE ONLY "1_platform_parameters" ADD CONSTRAINT "1_platform_parameters_pkey" PRIMARY KEY (id);
CREATE INDEX "1_platform_parameters_name_idx" ON "1_platform_parameters" (name);
DROP TABLE IF EXISTS "1_delayed_contracts";
CREATE TABLE "1_delayed_contracts" (
"id" bigint NOT NULL DEFAULT '0',
"contract" VARCHAR (255) NOT NULL DEFAULT '',
"key_id" bigint NOT NULL DEFAULT '0',
"block_id" bigint NOT NULL DEFAULT '0',
"every_block" bigint NOT NULL DEFAULT '0',
"counter" bigint NOT NULL DEFAULT '0',
"high_rate" bigint NOT NULL DEFAULT '0',
"limit" bigint NOT NULL DEFAULT '0',
"deleted" bigint NOT NULL DEFAULT '0',
"conditions" text NOT NULL DEFAULT ''
);
ALTER TABLE ONLY "1_delayed_contracts" ADD CONSTRAINT "1_delayed_contracts_pkey" PRIMARY KEY (id);
CREATE INDEX "1_delayed_contracts_block_id_idx" ON "1_delayed_contracts" (block_id);
DROP TABLE IF EXISTS "1_bad_blocks";
CREATE TABLE "1_bad_blocks" (
"id" bigint NOT NULL DEFAULT '0',
"producer_node_id" bigint NOT NULL DEFAULT '0',
"block_id" bigint NOT NULL DEFAULT '0',
"consumer_node_id" bigint NOT NULL DEFAULT '0',
"block_time" timestamp NOT NULL,
"reason" text NOT NULL DEFAULT '',
"deleted" bigint NOT NULL DEFAULT '0'
);
ALTER TABLE ONLY "1_bad_blocks" ADD CONSTRAINT "1_bad_blocks_pkey" PRIMARY KEY (id);
DROP TABLE IF EXISTS "1_node_ban_logs";
CREATE TABLE "1_node_ban_logs" (
"id" bigint NOT NULL DEFAULT '0',
"node_id" bigint NOT NULL DEFAULT '0',
"banned_at" timestamp NOT NULL,
"ban_time" bigint NOT NULL DEFAULT '0',
"reason" text NOT NULL DEFAULT ''
);
ALTER TABLE ONLY "1_node_ban_logs" ADD CONSTRAINT "1_node_ban_logs_pkey" PRIMARY KEY (id);
INSERT INTO "1_delayed_contracts"
		("id", "contract", "key_id", "block_id", "every_block", "high_rate", "conditions")
	VALUES
		(next_id('1_delayed_contracts'), '@1CheckNodesBan', '-1744264011260937456', '10', '10', '4','ContractConditions("@1MainCondition")');


INSERT INTO "1_ecosystems" ("id", "name", "is_valued", "digits", "token_symbol", "token_name") VALUES 
	(next_id('1_ecosystems'), 'platform ecosystem', '1', '0', '', '')
;

INSERT INTO "1_applications" (id, name, conditions, ecosystem) VALUES (next_id('1_applications'), 'System', 'ContractConditions("MainCondition")', '1');


INSERT INTO "1_contracts" (id, name, value, token_id, conditions, app_id, ecosystem)
VALUES
	(next_id('1_contracts'), 'AccessControlMode', 'contract AccessControlMode {
    data {
        VotingId int "optional"
    }

    func decentralizedAutonomous(){
        if !DBFind("@1ecosystems").Where({"id":$ecosystem_id,"control_mode":2}).Row(){
            warning "control mode DAO error"
        }
        var prev string
        prev = $stack[0]
        if Len($stack) > 3{
            prev = $stack[Len($stack) - 3]
        }
        if prev != "@1VotingDecisionCheck" {
            warning LangRes("@1contract_start_votingdecisioncheck_only")
        }

        $voting = DBFind("@1votings").Where({"ecosystem": $ecosystem_id, "id": $VotingId,"voting->name":{"$begin":"voting_for_control_mode_template"}}).Columns("voting->type_decision,flags->success,voting->type").Row()
        if Int($voting["voting.type"]) != 2 {
            warning LangRes("@1voting_type_invalid")
        }
        if Int($voting["voting.type_decision"]) != 4 {
            warning LangRes("@1voting_error_decision")
        }
        if Int($voting["flags.success"]) != 1 {
            warning LangRes("@1voting_error_success")
        }
    }
    func chooseControl(){
        $control = DBFind("@1ecosystems").Where({"id":$ecosystem_id,"control_mode":{"$in":["1","2"]}}).Row()
        if !$control{
            warning "control mode error"
        }

        if $control["control_mode"] == 2 && $VotingId{
            decentralizedAutonomous()
            return
        }
        DeveloperCondition()
    }
    conditions {
        $VotingId = Int($VotingId)
        chooseControl()
        $result = $control["control_mode"]
    }
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'AccessVoteTempRun', 'contract AccessVoteTempRun {
    data {
        ContractAccept string "optional"
        ContractAcceptParams map "optional"
    }

    func votingCheck(){
        var app_id int
        app_id = Int(DBFind("@1applications").Where({"ecosystem": $ecosystem_id, "name": "Basic"}).One("id"))
        $templateId = Int(DBFind("@1app_params").Where({"app_id": app_id, "name": "voting_template_control_mode", "ecosystem": $ecosystem_id}).One("value"))
        if $templateId == 0 {
            warning LangRes("@1template_id_not_found")
        }
    }

    action {
        votingCheck()
        var temp map
        temp["TemplateId"] = $templateId
        temp["Duration"] = 7
        temp["ContractAccept"] = $ContractAccept
        temp["ContractAcceptParams"] = JSONEncode($ContractAcceptParams)
        CallContract("@1VotingTemplateRun",temp)
    }
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'BindWallet', 'contract BindWallet {
	data {
		Id  int
	}
	conditions {
		$cur = DBRow("contracts").Columns("id,conditions,wallet_id").WhereId($Id)
		if !$cur {
			error Sprintf("Contract %d does not exist", $Id)
		}
		Eval($cur["conditions"])
		if $key_id != Int($cur["wallet_id"]) {
			error Sprintf("Wallet %d cannot activate the contract", $key_id)
		}
	}
	action {
		BndWallet($Id, $ecosystem_id)
	}
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'CallDelayedContract', 'contract CallDelayedContract {
	data {
        Id int
	}

	conditions {
		HonorNodeCondition()

		var rows array
		rows = DBFind("@1delayed_contracts").Where({"id": $Id, "deleted": 0})

		if !Len(rows) {
			warning Sprintf(LangRes("@1template_delayed_contract_not_exist"), $Id)
		}
		$cur = rows[0]
		$limit = Int($cur["limit"])
		$counter = Int($cur["counter"])

		if $block < Int($cur["block_id"]) {
			warning Sprintf(LangRes("@1template_delayed_contract_error"), $Id, $cur["block_id"], $block)
		}

		if $limit > 0 && $counter >= $limit {
			warning Sprintf(LangRes("@1template_delayed_contract_limited"), $Id)
		}
	}

	action {
		$counter = $counter + 1

		var block_id int
		block_id = $block
		if $limit == 0 || $limit > $counter {
			block_id = block_id + Int($cur["every_block"])
		}

		DBUpdate("@1delayed_contracts", $Id, {"counter": $counter, "block_id": block_id})

		var params map
		CallContract($cur["contract"], params)
	}
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'CheckNodesBan', 'contract CheckNodesBan {
    func getPermission() {
        var array_permissions array result i int prevContract string
        array_permissions = ["@1CheckNodesBan"]

        prevContract = $stack[0]
        if Len($stack) > 2 {
            prevContract = $stack[Len($stack) - 2]
        }
        while i < Len(array_permissions) {
            var contract_name string
            contract_name = array_permissions[i]
            if contract_name == prevContract {
                result = 1
            }
            i = i + 1
        }

        if result == 0 {
            warning LangRes("@1contract_chain_distorted")
        }
    }
    conditions {
        getPermission()
        HonorNodeCondition()
        var rows array
        rows = DBFind("@1delayed_contracts").Where({"contract": "@1CheckNodesBan", "deleted": 0})
        if !Len(rows) {
            warning Sprintf(LangRes("@1template_delayed_contract_not_exist"), $Id)
        }
        $cur = rows[0]
        $counter = Int($cur["counter"]) + 1
        $Id = Int($cur["id"])
    }
    action {
        DBUpdateExt("@1delayed_contracts", {"id":$Id}, {"counter": $counter})

        UpdateNodesBan($block_time)
    }
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'EditAppParam', 'contract EditAppParam {
    data {
        Id int
        Value string "optional"
        Conditions string "optional"
    }
    func onlyConditions() bool {
        return $Conditions && !$Value
    }

    conditions {
        RowConditions("app_params", $Id, onlyConditions())
        if $Conditions {
            ValidateCondition($Conditions, $ecosystem_id)
        }
    }

    action {
        var pars map
        if $Value {
            pars["value"] = $Value
        }
        if $Conditions {
            pars["conditions"] = $Conditions
        }
        if pars {
            DBUpdate("app_params", $Id, pars)
        }
    }
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'EditApplication', 'contract EditApplication {
    data {
        ApplicationId int
        Conditions string "optional"
    }
    func onlyConditions() bool {
        return $Conditions && false
    }

    conditions {
        RowConditions("applications", $ApplicationId, onlyConditions())
        if $Conditions {
            ValidateCondition($Conditions, $ecosystem_id)
        }
    }

    action {
        var pars map
        if $Conditions {
            pars["conditions"] = $Conditions
        }
        if pars {
            DBUpdate("applications", $ApplicationId, pars)
        }
    }
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'EditColumn', 'contract EditColumn {
    data {
        TableName string
        Name string
        Permissions string
    }

    conditions {
        ColumnCondition($TableName, $Name, "", $Permissions)
    }

    action {
        PermColumn($TableName, $Name, $Permissions)
    }
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'EditContract', 'contract EditContract {
    data {
        Id int
        Value string "optional"
        Conditions string "optional"
    }
    func onlyConditions() bool {
        return $Conditions && !$Value
    }

    conditions {
        RowConditions("contracts", $Id, onlyConditions())
        if $Conditions {
            ValidateCondition($Conditions, $ecosystem_id)
        }
        $cur = DBFind("contracts").Columns("id,value,conditions,wallet_id,token_id").WhereId($Id).Row()
        if !$cur {
            error Sprintf("Contract %d does not exist", $Id)
        }
        if $Value {
            ValidateEditContractNewValue($Value, $cur["value"])
        }
   
        $recipient = Int($cur["wallet_id"])
    }

    action {
        UpdateContract($Id, $Value, $Conditions, $recipient, $cur["token_id"])
    }
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'EditLang', 'contract EditLang {
    data {
        Id int
        Trans string
    }

    conditions {
        EvalCondition("parameters", "changing_language", "value")
        $lang = DBFind("languages").Where({id: $Id}).Row()
    }

    action {
        EditLanguage($Id, $lang["name"], $Trans)
    }
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'EditMenu', 'contract EditMenu {
    data {
        Id int
        Value string "optional"
        Title string "optional"
        Conditions string "optional"
    }
    func onlyConditions() bool {
        return $Conditions && !$Value && !$Title
    }

    conditions {
        RowConditions("menu", $Id, onlyConditions())
        if $Conditions {
            ValidateCondition($Conditions, $ecosystem_id)
        }
    }

    action {
        var pars map
        if $Value {
            pars["value"] = $Value
        }
        if $Title {
            pars["title"] = $Title
        }
        if $Conditions {
            pars["conditions"] = $Conditions
        }
        if pars {
            DBUpdate("menu", $Id, pars)
        }            
    }
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'EditPage', 'contract EditPage {
    data {
        Id int
        Value string "optional"
        Menu string "optional"
        Conditions string "optional"
        ValidateCount int "optional"
        ValidateMode string "optional"
    }
    func onlyConditions() bool {
        return $Conditions && !$Value && !$Menu && !$ValidateCount 
    }
    func preparePageValidateCount(count int) int {
        var min, max int
        min = Int(EcosysParam("min_page_validate_count"))
        max = Int(EcosysParam("max_page_validate_count"))
        if count < min {
            count = min
        } else {
            if count > max {
                count = max
            }
        }
        return count
    }

    conditions {
        RowConditions("pages", $Id, onlyConditions())
        if $Conditions {
            ValidateCondition($Conditions, $ecosystem_id)
        }
        $ValidateCount = preparePageValidateCount($ValidateCount)
    }

    action {
        var pars map
        if $Value {
            pars["value"] = $Value
        }
        if $Menu {
            pars["menu"] = $Menu
        }
        if $Conditions {
            pars["conditions"] = $Conditions
        }
        if $ValidateCount {
            pars["validate_count"] = $ValidateCount
        }
        if $ValidateMode {
            if $ValidateMode != "1" {
                $ValidateMode = "0"
            }
            pars["validate_mode"] = $ValidateMode
        }
        if pars {
            DBUpdate("pages", $Id, pars)
        }
    }
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'EditParameter', 'contract EditParameter {
    data {
        Id int
        Value string "optional"
        Conditions string "optional"
    }

    func onlyConditions() bool {
        return $Conditions && !$Value
    }

    conditions {
        DeveloperCondition()

        RowConditions("@1parameters", $Id, onlyConditions())
        if $Conditions {
            ValidateCondition($Conditions, $ecosystem_id)
        }
        if $Value {
            $Name = DBFind("@1parameters").Where({"id": $Id, "ecosystem": $ecosystem_id}).One("name")
            if $Name == "founder_account" {
                var account string
                account = IdToAddress(Int($Value))
                if !DBFind("@1keys").Where({"account": account, "ecosystem": $ecosystem_id, "deleted": 0}).One("id") {
                    warning Sprintf(LangRes("@1template_user_not_found"), $Value)
                }
            }
            if $Name == "max_block_user_tx" || $Name == "money_digit" || $Name == "max_sum" || $Name == "min_page_validate_count" || $Name == "max_page_validate_count" {
                if Size($Value) == 0 {
                    warning LangRes("@1value_not_received")
                }
                if Int($Value) <= 0 {
                    warning LangRes("@1value_must_greater_zero")
                }
            }
        }
    }

    action {
        var pars map
        if $Value {
            if $Value == `""` {
                pars["value"] = ""
            } else {
                pars["value"] = $Value
            }
        }
        if $Conditions {
            pars["conditions"] = $Conditions
        }
        if pars {
            DBUpdate("@1parameters", $Id, pars)
        }
    }
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'EditSnippet', 'contract EditSnippet {
    data {
        Id int
        Value string "optional"
        Conditions string "optional"
    }
    func onlyConditions() bool {
        return $Conditions && !$Value
    }

    conditions {
        RowConditions("snippets", $Id, onlyConditions())
        if $Conditions {
            ValidateCondition($Conditions, $ecosystem_id)
        }
    }

    action {
        var pars map
        if $Value {
            pars["value"] = $Value
        }
        if $Conditions {
            pars["conditions"] = $Conditions
        }
        if pars {
            DBUpdate("snippets", $Id, pars)
        }
    }
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'EditTable', 'contract EditTable {
    data {
        Name string
        InsertPerm string
        UpdatePerm string
        NewColumnPerm string
        ReadPerm string "optional"
    }

    conditions {
        if !$InsertPerm {
            info("Insert condition is empty")
        }
        if !$UpdatePerm {
            info("Update condition is empty")
        }
        if !$NewColumnPerm {
            info("New column condition is empty")
        }

        var permissions map
        permissions["insert"] = $InsertPerm
        permissions["update"] = $UpdatePerm
        permissions["new_column"] = $NewColumnPerm
        if $ReadPerm {
            permissions["read"] = $ReadPerm
        }
        $Permissions = permissions
        TableConditions($Name, "", JSONEncode($Permissions))
    }

    action {
        PermTable($Name, JSONEncode($Permissions))
    }
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'HonorNodeCondition', 'contract HonorNodeCondition {
	conditions {
		var account_key int
		account_key = AddressToId($account_id)
		if IsHonorNodeKey(account_key) {
			return
		}
		warning "HonorNodeCondition: Sorry, you do not have access to this action"
	}
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'Import', 'contract Import {
    data {
        Data string
    }
    conditions {
        $ApplicationId = 0
        var app_map map
        app_map = DBFind("@1buffer_data").Columns("value->app_name").Where({"key": "import_info", "account": $account_id, "ecosystem": $ecosystem_id}).Row()
        if app_map {
            var app_id int ival string
            ival = Str(app_map["value.app_name"])
            app_id = Int(DBFind("@1applications").Columns("id").Where({"name": ival, "ecosystem": $ecosystem_id}).One("id"))
            if app_id {
                $ApplicationId = app_id
            }
        }
    }
    action {
        var editors, creators map
        editors["pages"] = "EditPage"
        editors["snippets"] = "EditSnippet"
        editors["menu"] = "EditMenu"
        editors["app_params"] = "EditAppParam"
        editors["languages"] = "EditLang"
        editors["contracts"] = "EditContract"
        editors["tables"] = "" // nothing
        creators["pages"] = "NewPage"
        creators["snippets"] = "NewSnippet"
        creators["menu"] = "NewMenu"
        creators["app_params"] = "NewAppParam"
        creators["languages"] = "NewLang"
        creators["contracts"] = "NewContract"
        creators["tables"] = "NewTable"
        var dataImport array
        dataImport = JSONDecode($Data)
        var i int
        while i < Len(dataImport) {
            var item cdata map type name string
            cdata = dataImport[i]
            if cdata {
                cdata["ApplicationId"] = $ApplicationId
                type = cdata["Type"]
                name = cdata["Name"]
                // Println(Sprintf("import %v: %v", type, cdata["Name"]))
                var tbl string
                tbl = "@1" + Str(type)
                if type == "app_params" {
                    item = DBFind(tbl).Where({"name": name, "ecosystem": $ecosystem_id, "app_id": $ApplicationId}).Row()
                } else {
                    item = DBFind(tbl).Where({"name": name, "ecosystem": $ecosystem_id}).Row()
                }
                var contractName string
                if item {
                    contractName = editors[type]
                    cdata["Id"] = Int(item["id"])
                    if type == "contracts" {
                        if item["conditions"] == "false" {
                            // ignore updating impossible
                            contractName = ""
                        }
                    } elif type == "menu" {
                        var menu menuItem string
                        menu = Replace(item["value"], " ", "")
                        menu = Replace(menu, "\n", "")
                        menu = Replace(menu, "\r", "")
                        menuItem = Replace(cdata["Value"], " ", "")
                        menuItem = Replace(menuItem, "\n", "")
                        menuItem = Replace(menuItem, "\r", "")
                        if Contains(menu, menuItem) {
                            // ignore repeated
                            contractName = ""
                        } else {
                            cdata["Value"] = item["value"] + "\n" + cdata["Value"]
                        }
                    }
                } else {
                    contractName = creators[type]
                }
                if contractName != "" {
                    CallContract(contractName, cdata)
                }
            }
            i = i + 1
        }
        // Println(Sprintf("> time: %v", $time))
    }
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'ImportUpload', 'contract ImportUpload {
    data {
        Data file
    }
    conditions {
        $Body = BytesToString($Data["Body"])
        $limit = 10 // data piece size of import
    }
    action {
        // init buffer_data, cleaning old buffer
        var initJson map
        $import_id = Int(DBFind("@1buffer_data").Where({"account": $account_id, "key": "import", "ecosystem": $ecosystem_id}).One("id"))
        if $import_id {
             DBUpdate("@1buffer_data", $import_id, {"value": initJson})
        } else {
            $import_id = DBInsert("@1buffer_data", {"account": $account_id, "key": "import", "value": initJson, "ecosystem": $ecosystem_id})
        }
        $info_id = Int(DBFind("@1buffer_data").Where({"account": $account_id, "key": "import_info", "ecosystem": $ecosystem_id}).One("id"))
        if $info_id {
            DBUpdate("@1buffer_data", $info_id, {"value": initJson})
        } else {
            $info_id = DBInsert("@1buffer_data", {"account": $account_id, "key": "import_info", "value": initJson, "ecosystem": $ecosystem_id})
        }
        var input map arrData array
        input = JSONDecode($Body)
        arrData = input["data"]
        var pages_arr blocks_arr menu_arr parameters_arr languages_arr contracts_arr tables_arr array
        // IMPORT INFO
        var i lenArrData int item map
        lenArrData = Len(arrData)
        while i < lenArrData {
            item = arrData[i]
            if item["Type"] == "pages" {
                pages_arr = Append(pages_arr, item["Name"])
            } elif item["Type"] == "snippets" {
                blocks_arr = Append(blocks_arr, item["Name"])
            } elif item["Type"] == "menu" {
                menu_arr = Append(menu_arr, item["Name"])
            } elif item["Type"] == "app_params" {
                parameters_arr = Append(parameters_arr, item["Name"])
            } elif item["Type"] == "languages" {
                languages_arr = Append(languages_arr, item["Name"])
            } elif item["Type"] == "contracts" {
                contracts_arr = Append(contracts_arr, item["Name"])
            } elif item["Type"] == "tables" {
                tables_arr = Append(tables_arr, item["Name"])
            }
            i = i + 1
        }
        var inf map
        inf["app_name"] = input["name"]
        inf["pages"] = Join(pages_arr, ", ")
        inf["pages_count"] = Len(pages_arr)
        inf["snippets"] = Join(blocks_arr, ", ")
        inf["blocks_count"] = Len(blocks_arr)
        inf["menu"] = Join(menu_arr, ", ")
        inf["menu_count"] = Len(menu_arr)
        inf["parameters"] = Join(parameters_arr, ", ")
        inf["parameters_count"] = Len(parameters_arr)
        inf["languages"] = Join(languages_arr, ", ")
        inf["languages_count"] = Len(languages_arr)
        inf["contracts"] = Join(contracts_arr, ", ")
        inf["contracts_count"] = Len(contracts_arr)
        inf["tables"] = Join(tables_arr, ", ")
        inf["tables_count"] = Len(tables_arr)
        if 0 == inf["pages_count"] + inf["blocks_count"] + inf["menu_count"] + inf["parameters_count"] + inf["languages_count"] + inf["contracts_count"] + inf["tables_count"] {
            warning "Invalid or empty import file"
        }
        // IMPORT DATA
        // the contracts is imported in one piece, the rest is cut under the $limit
        var sliced contracts array
        i = 0
        while i < lenArrData {
            var items array l int item map
            while l < $limit && (i + l < lenArrData) {
                item = arrData[i + l]
                if item["Type"] == "contracts" {
                    contracts = Append(contracts, item)
                } else {
                    items = Append(items, item)
                }
                l = l + 1
            }
            var batch map
            batch["Data"] = JSONEncode(items)
            sliced = Append(sliced, batch)
            i = i + $limit
        }
        if Len(contracts) > 0 {
            var batch map
            batch["Data"] = JSONEncode(contracts)
            sliced = Append(sliced, batch)
        }
        input["data"] = sliced
        // storing
        DBUpdate("@1buffer_data", $import_id, {"value": input})
        DBUpdate("@1buffer_data", $info_id, {"value": inf})
        var name string
        name = Str(input["name"])
        var cndns string
        cndns = Str(input["conditions"])
        if !DBFind("@1applications").Columns("id").Where({"name": name, "ecosystem": $ecosystem_id}).One("id") {
            DBInsert("@1applications", {"name": name, "conditions": cndns, "ecosystem": $ecosystem_id})
        }
    }
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'NewAppParam', 'contract NewAppParam {
    data {
        ApplicationId int
        Name string
        Value string
        Conditions string
    }

    conditions {
        ValidateCondition($Conditions, $ecosystem_id)

        if $ApplicationId == 0 {
            warning "Application id cannot equal 0"
        }

        if DBFind("app_params").Columns("id").Where({"name":$Name}).One("id") {
            warning Sprintf( "Application parameter %s already exists", $Name)
        }
    }

    action {
        DBInsert("app_params", {app_id: $ApplicationId, name: $Name, value: $Value,
              conditions: $Conditions})
    }
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'NewApplication', 'contract NewApplication {
    data {
        Name string
        Conditions string
    }

    conditions {
        ValidateCondition($Conditions, $ecosystem_id)

        if Size($Name) == 0 {
            warning "Application name missing"
        }

        if DBFind("applications").Columns("id").Where({name:$Name}).One("id") {
            warning Sprintf( "Application %s already exists", $Name)
        }
    }

    action {
        $result = DBInsert("applications", {name: $Name,conditions: $Conditions})
    }
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'NewBadBlock', 'contract NewBadBlock {
	data {
		ProducerNodeID int
		ConsumerNodeID int
		BlockID int
		Timestamp int
		Reason string
	}
	action {
        DBInsert("@1bad_blocks", {producer_node_id: $ProducerNodeID,consumer_node_id: $ConsumerNodeID,
            block_id: $BlockID, "timestamp block_time": $Timestamp, reason: $Reason})
	}
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'NewContract', 'contract NewContract {
    data {
        ApplicationId int
        Value string
        Conditions string
        TokenEcosystem int "optional"
    }

    conditions {
        ValidateCondition($Conditions,$ecosystem_id)

        if $ApplicationId == 0 {
            warning "Application id cannot equal 0"
        }

        $contract_name = ContractName($Value)

        if !$contract_name {
            error "must be the name"
        }

        if !$TokenEcosystem {
            $TokenEcosystem = 1
        } else {
            if !SysFuel($TokenEcosystem) {
                warning Sprintf("Ecosystem %d is not system", $TokenEcosystem)
            }
        }
    }

    action {
        $result = CreateContract($contract_name, $Value, $Conditions, $TokenEcosystem, $ApplicationId)
    }
    func price() int {
        return SysParamInt("contract_price")
    }
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'NewEcosystem', 'contract NewEcosystem {
	data {
		Name  string
	}
	action {
		$result = CreateEcosystem($key_id, $Name)
	}
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'NewLang', 'contract NewLang {
    data {
        ApplicationId int
        Name string
        Trans string
    }

    conditions {
        if $ApplicationId == 0 {
            warning "Application id cannot equal 0"
        }

        if DBFind("languages").Columns("id").Where({name: $Name}).One("id") {
            warning Sprintf( "Language resource %s already exists", $Name)
        }

        EvalCondition("parameters", "changing_language", "value")
    }

    action {
        CreateLanguage($Name, $Trans)
    }
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'NewMenu', 'contract NewMenu {
    data {
        Name string
        Value string
        Title string "optional"
        Conditions string
    }

    conditions {
        ValidateCondition($Conditions,$ecosystem_id)

        if DBFind("menu").Columns("id").Where({name: $Name}).One("id") {
            warning Sprintf( "Menu %s already exists", $Name)
        }
    }

    action {
        DBInsert("menu", {name:$Name,value: $Value, title: $Title, conditions: $Conditions})
    }
    func price() int {
        return SysParamInt("menu_price")
    }
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'NewPage', 'contract NewPage {
    data {
        ApplicationId int
        Name string
        Value string
        Menu string
        Conditions string
        ValidateCount int "optional"
        ValidateMode string "optional"
    }
    func preparePageValidateCount(count int) int {
        var min, max int
        min = Int(EcosysParam("min_page_validate_count"))
        max = Int(EcosysParam("max_page_validate_count"))

        if count < min {
            count = min
        } else {
            if count > max {
                count = max
            }
        }
        return count
    }

    conditions {
        ValidateCondition($Conditions,$ecosystem_id)

        if $ApplicationId == 0 {
            warning "Application id cannot equal 0"
        }

        if DBFind("pages").Columns("id").Where({name: $Name}).One("id") {
            warning Sprintf( "Page %s already exists", $Name)
        }

        $ValidateCount = preparePageValidateCount($ValidateCount)

        if $ValidateMode {
            if $ValidateMode != "1" {
                $ValidateMode = "0"
            }
        }
    }

    action {
        DBInsert("pages", {name: $Name,value: $Value, menu: $Menu,
             validate_count:$ValidateCount,validate_mode: $ValidateMode,
             conditions: $Conditions,app_id: $ApplicationId})
    }
    func price() int {
        return SysParamInt("page_price")
    }
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'NewParameter', 'contract NewParameter {
    data {
        Name string
        Value string
        Conditions string
    }
    func warnEmpty(name value string) {
        if Size(value) == 0 {
            warning Sprintf(LangRes("@1x_parameter_empty"),name)
        }
    }
    conditions {
        DeveloperCondition()

        ValidateCondition($Conditions, $ecosystem_id)
        $Name = TrimSpace($Name)
        warnEmpty("Name",$Name)
        if DBFind("@1parameters").Where({"name": $Name, "ecosystem": $ecosystem_id}).One("id") {
            warning Sprintf(LangRes("@1template_parameter_exists"), $Name)
        }
    }

    action {
        DBInsert("@1parameters", {"name": $Name, "value": $Value, "conditions": $Conditions, "ecosystem": $ecosystem_id})
    }
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'NewSnippet', 'contract NewSnippet {
    data {
        ApplicationId int
        Name string
        Value string
        Conditions string
    }

    conditions {
        ValidateCondition($Conditions, $ecosystem_id)

        if $ApplicationId == 0 {
            warning "Application id cannot equal 0"
        }

        if DBFind("snippets").Columns("id").Where({name:$Name}).One("id") {
            warning Sprintf( "Block %s already exists", $Name)
        }
    }

    action {
        DBInsert("snippets", {name: $Name, value: $Value, conditions: $Conditions,
              app_id: $ApplicationId})
    }
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'NewTable', 'contract NewTable {
    data {
        ApplicationId int
        Name string
        Columns string
        Permissions string
    }
    conditions {
        if $ApplicationId == 0 {
            warning "Application id cannot equal 0"
        }
        TableConditions($Name, $Columns, $Permissions)
    }
    
    action {
        CreateTable($Name, $Columns, $Permissions, $ApplicationId)
    }
    func price() int {
        return SysParamInt("table_price")
    }
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'NewUser', 'contract NewUser {
    data {
        NewPubkey string "optional"
        Ecosystem int "optional"
    }
    func getEcosystem() {
        $e_id = Int($Ecosystem)
        if $e_id == 0 {
            $e_id = $ecosystem_id
        }
        $eco = DBFind("@1ecosystems").Where({"id": $e_id}).Row()
        if !$eco {
            warning Sprintf(LangRes("@1ecosystem_not_found"), $e_id)
        }
    }
    func canOpt() bool {
        return $free_membership == 1 || $e_id == 1
    }
    conditions {
        getEcosystem()
        $newId = PubToID($NewPubkey)

        if $newId == 0 {
            warning LangRes("@1wrong_pub")
        }
        if Size($NewPubkey) == 0 {
            warning "You did not enter the public key"
        }
        $pub = HexToPub($NewPubkey)
        $account = IdToAddress($newId)

        $k = DBFind("@1keys").Where({"id": $newId, "account": $account, "ecosystem": $e_id}).Row()

        $free_membership = Int(DBFind("@1parameters").Where({"ecosystem": $e_id, "name": "free_membership"}).One("value"))
    }

    action {
        var iscan bool
        iscan = canOpt()
        if !iscan {
            warning Sprintf(LangRes("@1eco_no_open_new_user"), $eco["name"], $e_id)
        }
        if $k {
            var kid int kpub string
            kid = Int($k["id"])
            kpub = $k["pub"]
            if Size(kpub) != 0 {
                warning Sprintf(LangRes("@1template_user_exists"), IdToAddress($newId))
            }
            DBUpdateExt("@1keys", {"id": kid, "ecosystem": $e_id}, {"pub": $pub})
            $result = $account
        }else{
            DBInsert("@1keys", {"id": $newId, "account": $account, "pub": $pub, "ecosystem": $e_id})
            if !DBFind("@1keys").Where({"ecosystem": 1, "account": $account}).Row() {
                DBInsert("@1keys", {"id": $newId, "account": $account, "pub": $pub, "ecosystem": 1})
                var h map
                $whiteHoleBalance = DBFind("@1keys").Where({"account": $white_hole_account,"ecosystem":1}).Columns("amount").One("amount")
                h["sender_id"] =$white_hole_key
                h["sender_balance"] = $whiteHoleBalance
                h["recipient_id"] = $newId
                h["comment"] = "New User"
                h["block_id"] = $block
                h["txhash"] = $txhash
                h["ecosystem"] = 1
                h["type"] = 4
                h["created_at"] = $time
                DBInsert("@1history", h)
            }
            $result = $account
        }
        var h map
        $whiteHoleBalance = DBFind("@1keys").Where({"account": $white_hole_account,"ecosystem":$ecosystem_id}).Columns("amount").One("amount")
        h["sender_id"] =$white_hole_key
        h["sender_balance"] = $whiteHoleBalance
        h["recipient_id"] = $newId
        h["comment"] = "New User"
        h["block_id"] = $block
        h["txhash"] = $txhash
        h["ecosystem"] = $ecosystem_id
        h["type"] = 4
        h["created_at"] = $time
        DBInsert("@1history", h)
    }
}
', '1', 'ContractConditions("NodeOwnerCondition")', '1', '1'),
	(next_id('1_contracts'), 'NodeOwnerCondition', 'contract NodeOwnerCondition {
	conditions {
        $raw_honor_nodes = SysParamString("honor_nodes")
        if Size($raw_honor_nodes) == 0 {
            ContractConditions("MainCondition")
        } else {
            $honor_nodes = JSONDecode($raw_honor_nodes)
            var i int
            while i < Len($honor_nodes) {
                $fn = $honor_nodes[i]
                if $fn["key_id"] == $key_id {
                    return true
                }
                i = i + 1
            }
            warning "NodeOwnerCondition: Sorry, you do not have access to this action."
        }
	}
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'UnbindWallet', 'contract UnbindWallet {
	data {
		Id         int
	}
	conditions {
		$cur = DBRow("contracts").Columns("id,conditions,wallet_id").WhereId($Id)
		if !$cur {
			error Sprintf("Contract %d does not exist", $Id)
		}
		
		Eval($cur["conditions"])
		if $key_id != Int($cur["wallet_id"]) {
			error Sprintf("Wallet %d cannot deactivate the contract", $key_id)
		}
	}
	action {
		UnbndWallet($Id, $ecosystem_id)
	}
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'UpdatePlatformParam', 'contract UpdatePlatformParam {
     data {
        Name string
        Value string
        Conditions string "optional"
     }
     conditions {
         if !GetContractByName($Name){
            warning "System parameter not found"
         }
     }
     action {
        var params map
        params["Value"] = $Value
        CallContract($Name, params)
        
        DBUpdatePlatformParam($Name, $Value, $Conditions)
     }
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'UploadBinary', 'contract UploadBinary {
    data {
        ApplicationId int
        Name string
        Data bytes
        DataMimeType string "optional"
        MemberAccount string "optional"
    }
    conditions {
        if Size($MemberAccount) > 0 {
            $UserID = $MemberAccount
        } else {
            $UserID = $account_id
        }
        $Id = Int(DBFind("@1binaries").Columns("id").Where({"app_id": $ApplicationId, 
                "account": $UserID, "name": $Name, "ecosystem": $ecosystem_id}).One("id"))
        if $Id == 0 {
            if $ApplicationId == 0 {
                warning LangRes("@1aid_cannot_zero")
            }
        }
    }
    action {
        var hash string
        hash = Hash($Data)
        if $DataMimeType == "" {
            $DataMimeType = "application/octet-stream"
        }
        if $Id != 0 {
            DBUpdate("@1binaries", $Id, {"data": $Data, "hash": hash, "mime_type": $DataMimeType})
        } else {
            $Id = DBInsert("@1binaries", {"app_id": $ApplicationId, "account": $UserID,
                "name": $Name, "data": $Data, "hash": hash, "mime_type": $DataMimeType, "ecosystem": $ecosystem_id})
        }
        $result = $Id
    }
}
', '1', 'ContractConditions("MainCondition")', '1', '1'),
	(next_id('1_contracts'), 'UploadFile', 'contract UploadFile {
    data {
        ApplicationId int
        Data file
        Name string "optional"
    }
    conditions {
        if $Name == "" {
            $Name = $Data["Name"]
        }
        $Body = $Data["Body"]
        $DataMimeType = $Data["MimeType"]
    }
    action {
        $Id = @1UploadBinary("ApplicationId,Name,Data,DataMimeType", $ApplicationId, $Name, $Body, $DataMimeType)
        $result = $Id
    }
}
', '1', 'ContractConditions("MainCondition")', '1', '1');

INSERT INTO "1_pages" (id, name, value, menu, conditions, app_id, ecosystem) VALUES
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

INSERT INTO "1_snippets" (id, name, value, conditions, app_id, ecosystem) VALUES
		(next_id('1_snippets'), 'pager_header', '', 'ContractConditions("@1DeveloperCondition")', '1', '1');


INSERT INTO "1_platform_parameters" ("id","name", "value", "conditions") VALUES 
	(next_id('1_platform_parameters'),'default_ecosystem_page', 'If(#ecosystem_id# > 1){Include(@1welcome)}', 'ContractAccess("@1UpdatePlatformParam")'),
	(next_id('1_platform_parameters'),'default_ecosystem_menu', '', 'ContractAccess("@1UpdatePlatformParam")'),
	(next_id('1_platform_parameters'),'default_ecosystem_contract', '', 'ContractAccess("@1UpdatePlatformParam")'),
	(next_id('1_platform_parameters'),'gap_between_blocks', '2', 'ContractAccess("@1UpdatePlatformParam")'),
	(next_id('1_platform_parameters'),'rollback_blocks', '60', 'ContractAccess("@1UpdatePlatformParam")'),
	(next_id('1_platform_parameters'),'honor_nodes', '', 'ContractAccess("@1UpdatePlatformParam")'),
	(next_id('1_platform_parameters'),'number_of_nodes', '101', 'ContractAccess("@1UpdatePlatformParam")'),
	(next_id('1_platform_parameters'),'max_block_size', '67108864', 'ContractAccess("@1UpdatePlatformParam")'),
	(next_id('1_platform_parameters'),'max_tx_size', '33554432', 'ContractAccess("@1UpdatePlatformParam")'),
	(next_id('1_platform_parameters'),'max_tx_block', '5000', 'ContractAccess("@1UpdatePlatformParam")'),
	(next_id('1_platform_parameters'),'max_columns', '50', 'ContractAccess("@1UpdatePlatformParam")'),
	(next_id('1_platform_parameters'),'max_indexes', '5', 'ContractAccess("@1UpdatePlatformParam")'),
	(next_id('1_platform_parameters'),'max_tx_block_per_user', '5000', 'ContractAccess("@1UpdatePlatformParam")'),
	(next_id('1_platform_parameters'),'max_fuel_tx', '20000000', 'ContractAccess("@1UpdatePlatformParam")'),
	(next_id('1_platform_parameters'),'max_fuel_block', '200000000', 'ContractAccess("@1UpdatePlatformParam")'),
	(next_id('1_platform_parameters'),'taxes_size', '3', 'ContractAccess("@1UpdatePlatformParam")'),
	(next_id('1_platform_parameters'),'taxes_wallet', '', 'ContractAccess("@1UpdatePlatformParam")'),
	(next_id('1_platform_parameters'),'fuel_rate', '[["1","1000000"]]', 'ContractAccess("@1UpdatePlatformParam")'),
	(next_id('1_platform_parameters'),'max_block_generation_time', '2000', 'ContractAccess("@1UpdatePlatformParam")'),
	(next_id('1_platform_parameters'),'incorrect_blocks_per_day','10','ContractAccess("@1UpdatePlatformParam")'),
	(next_id('1_platform_parameters'),'node_ban_time','86400000','ContractAccess("@1UpdatePlatformParam")'),
	(next_id('1_platform_parameters'),'node_ban_time_local','1800000','ContractAccess("@1UpdatePlatformParam")'),
	(next_id('1_platform_parameters'),'price_tx_size', '15', 'ContractAccess("@1UpdatePlatformParam")'),
	(next_id('1_platform_parameters'),'test','false','false'),
	(next_id('1_platform_parameters'),'price_tx_data', '10', 'ContractAccess("@1UpdatePlatformParam")'),
	(next_id('1_platform_parameters'),'private_blockchain', '1', 'false'),
	(next_id('1_platform_parameters'),'pay_free_contract', '@1CallDelayedContract,@1CheckNodesBan,@1NewUser', 'ContractAccess("@1UpdatePlatformParam")'),
    (next_id('1_platform_parameters'),'local_node_ban_time', '60', 'ContractAccess("@1UpdatePlatformParam")');


INSERT INTO "1_tables" ("id", "name", "permissions","columns", "conditions") VALUES
    (next_id('1_tables'), 'delayed_contracts',
        '{
            "read": "true",
            "insert": "ContractAccess(\"@1NewDelayedContract\")",
            "update": "ContractAccess(\"@1CallDelayedContract\",\"@1EditDelayedContract\",\"@1CheckNodesBan\")",
            "new_column": "ContractConditions(\"@1MainCondition\")"
        }',
        '{
            "contract": "ContractAccess(\"@1EditDelayedContract\")",
            "key_id": "ContractAccess(\"@1EditDelayedContract\")",
            "block_id": "ContractAccess(\"@1CallDelayedContract\",\"@1EditDelayedContract\")",
            "every_block": "ContractAccess(\"@1EditDelayedContract\")",
            "counter": "ContractAccess(\"@1CallDelayedContract\",\"@1EditDelayedContract\",\"@1CheckNodesBan\")",
            "high_rate": "ContractAccess(\"@1EditDelayedContract\")",
            "limit": "ContractAccess(\"@1EditDelayedContract\")",
            "deleted": "ContractAccess(\"@1EditDelayedContract\")",
            "conditions": "ContractAccess(\"@1EditDelayedContract\")"
        }',
        'ContractConditions("@1MainCondition")'
    ),
    (next_id('1_tables'), 'ecosystems',
        '{
            "read": "true",
            "insert": "ContractAccess(\"@1NewEcosystem\")",
            "update": "ContractAccess(\"@1EditEcosystemName\",\"@1VotingVesAccept\",\"@1EcManageInfo\",\"@1EcoFeeModeManage\",\"@1EditControlMode\",\"@1NewToken\",\"@1TeChange\",\"@1TeEmission\",\"@1TeBurn\")",
            "new_column": "ContractConditions(\"@1MainCondition\")"
        }',
        '{
            "name": "ContractAccess(\"@1EditEcosystemName\")",
            "info": "ContractAccess(\"@1EcManageInfo\")",
            "digits": "ContractAccess(\"@1NewToken\")",
            "fee_mode_info": "ContractAccess(\"@1EcoFeeModeManage\")",
            "is_valued": "ContractAccess(\"@1VotingVesAccept\")",
            "emission_amount": "ContractAccess(\"@1NewToken\",\"@1TeBurn\",\"@1TeEmission\")",
            "token_symbol": "ContractAccess(\"@1NewToken\")",
            "token_name": "ContractAccess(\"@1NewToken\")",
            "type_emission": "ContractAccess(\"@1TeChange\",\"@1NewToken\")",
            "type_withdraw": "ContractAccess(\"@1TeChange\",\"@1NewToken\")",
            "control_mode": "ContractAccess(\"@1EditControlMode\")"
        }',
        'ContractConditions("@1MainCondition")'
    ),
    (next_id('1_tables'), 'platform_parameters',
        '{
            "read": "true",
            "insert": "false",
            "update": "ContractAccess(\"@1UpdatePlatformParam\")",
            "new_column": "ContractConditions(\"@1MainCondition\")"
        }',
        '{
            "value": "ContractAccess(\"@1UpdatePlatformParam\")",
            "name": "false",
            "conditions": "ContractAccess(\"@1UpdatePlatformParam\")"
        }',
        'ContractConditions("@1MainCondition")'
    ),
    (next_id('1_tables'), 'bad_blocks',
        '{
            "read": "true",
            "insert": "ContractAccess(\"@1NewBadBlock\")",
            "update": "ContractAccess(\"@1NewBadBlock\", \"@1CheckNodesBan\")",
            "new_column": "ContractConditions(\"@1MainCondition\")"
        }',
        '{
            "block_id": "ContractAccess(\"@1CheckNodesBan\")",
            "producer_node_id": "ContractAccess(\"@1CheckNodesBan\")",
            "consumer_node_id": "ContractAccess(\"@1CheckNodesBan\")",
            "block_time": "ContractAccess(\"@1CheckNodesBan\")",
            "reason": "ContractAccess(\"@1CheckNodesBan\")",
            "deleted": "ContractAccess(\"@1CheckNodesBan\")"
        }',
        'ContractConditions("@1MainCondition")'
    ),
    (next_id('1_tables'), 'node_ban_logs',
        '{
            "read": "true",
            "insert": "ContractAccess(\"@1CheckNodesBan\")",
            "update": "ContractAccess(\"@1CheckNodesBan\")",
            "new_column": "ContractConditions(\"@1MainCondition\")"
        }',
        '{
            "node_id": "ContractAccess(\"@1CheckNodesBan\")",
            "banned_at": "ContractAccess(\"@1CheckNodesBan\")",
            "ban_time": "ContractAccess(\"@1CheckNodesBan\")",
            "reason": "ContractAccess(\"@1CheckNodesBan\")"
        }',
        'ContractConditions("@1MainCondition")'
    ),
    (next_id('1_tables'), 'time_zones',
        '{
            "read": "true",
            "insert": "false",
            "update": "false",
            "new_column": "false"
        }',
        '{
            "name": "false",
            "offset": "false"
        }',
        'ContractConditions("@1MainCondition")'
    );
