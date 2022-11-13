/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

var firstTablesDataSQL = `
INSERT INTO "1_tables" ("id", "name", "permissions","columns", "conditions") VALUES
    (next_id('1_tables'), 'delayed_contracts',
        '{
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
            "insert": "ContractAccess(\"@1NewEcosystem\")",
            "update": "ContractAccess(\"@1EditEcosystemName\",\"@1VotingVesAccept\",\"@1EcManageInfo\",\"@1EcoFeeModeManage\",\"@1EditControlMode\",\"@1NewToken\",\"@1TeChange\",\"@1TeEmission\",\"@1TeBurn\")",
            "new_column": "ContractConditions(\"@1MainCondition\")"
        }',
        '{
            "name": "ContractAccess(\"@1EditEcosystemName\")",
            "info": "ContractAccess(\"@1EcManageInfo\")",
            "fee_mode_info": "ContractAccess(\"@1EcoFeeModeManage\")",
            "is_valued": "ContractAccess(\"@1VotingVesAccept\")",
            "emission_amount": "ContractAccess(\"@1NewToken\",\"@1TeBurn\",\"@1TeEmission\")",
            "token_symbol": "ContractAccess(\"@1NewToken\")",
            "token_name": "ContractAccess(\"@1NewToken\")",
            "type_emission": "ContractAccess(\"@1TeChange\")",
            "type_withdraw": "ContractAccess(\"@1TeChange\")",
            "control_mode": "ContractAccess(\"@1EditControlMode\")"
        }',
        'ContractConditions("@1MainCondition")'
    ),
    (next_id('1_tables'), 'platform_parameters',
        '{
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
`
