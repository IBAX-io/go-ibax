/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

var tablesDataSQL = `INSERT INTO "1_tables" ("id", "name", "permissions","columns", "conditions", "ecosystem") VALUES
    (next_id('1_tables'), 'contracts',
        '{
            "insert": "ContractConditions(\"DeveloperCondition\")",
            "update": "ContractConditions(\"DeveloperCondition\")",
            "new_column": "ContractConditions(\"@1MainCondition\")"
        }',
        '{    
            "name": "false",
            "value": "ContractAccess(\"@1EditContract\")",
            "wallet_id": "ContractAccess(\"@1BindWallet\", \"@1UnbindWallet\")",
            "token_id": "ContractAccess(\"@1EditContract\")",
            "conditions": "ContractAccess(\"@1EditContract\")",
            "permissions": "ContractConditions(\"@1MainCondition\")",
            "app_id": "ContractAccess(\"@1ItemChangeAppId\")",
            "ecosystem": "false"
        }',
        'ContractConditions("@1MainCondition")', '{{.Ecosystem}}'
    ),
    (next_id('1_tables'), 'keys',
        '{
            "insert": "true",
            "update": "ContractAccess(\"@1TokensTransfer\",\"@1TokensLockoutMember\",\"@1NewToken\",\"@1TeBurn\",\"@1ProfileEdit\",\"@1NewUser\")",
            "new_column": "ContractConditions(\"@1MainCondition\")"
        }',
        '{
            "pub": "ContractAccess(\"@1NewUser\")",
            "amount": "ContractAccess(\"@1TokensTransfer\",\"@1NewToken\",\"@1TeBurn\",\"@1ProfileEdit\")",
            "maxpay": "ContractConditions(\"@1MainCondition\")",
            "deleted": "ContractAccess(\"@1DeleteMember\")",
            "blocked": "ContractAccess(\"@1TokensLockoutMember\")",
            "account": "false",
            "ecosystem": "false",
            "multi": "ContractConditions(\"@1MainCondition\")"
        }',
        'ContractConditions("@1MainCondition")', '{{.Ecosystem}}'
    ),
    (next_id('1_tables'), 'history',
        '{
            "insert": "ContractAccess(\"@1TokensTransfer\",\"@1NewUser\",\"@1NewToken\",\"@1TeBurn\",\"@1ProfileEdit\",\"@1MembershipRequest\",\"@1MembershipDecide\",\"@1MembershipAdd\",\"@1DeleteMember\")",
            "update": "ContractConditions(\"@1MainCondition\")",
            "new_column": "ContractConditions(\"@1MainCondition\")"
        }',
        '{
            "sender_id": "false",
            "recipient_id": "false",
            "sender_balance": "false",
            "recipient_balance": "false",
            "amount":  "false",
            "value_detail":  "false",
            "comment": "false",
            "status": "false",
            "block_id":  "false",
            "txhash": "false",
            "ecosystem": "false",
            "type": "false",
            "created_at": "false"
        }',
        'ContractConditions("@1MainCondition")', '{{.Ecosystem}}'
    ),
    (next_id('1_tables'), 'languages',
        '{
            "insert": "ContractConditions(\"DeveloperCondition\")",
            "update": "ContractConditions(\"DeveloperCondition\")",
            "new_column": "ContractConditions(\"@1MainCondition\")"
        }',
        '{
            "name": "ContractAccess(\"@1EditLang\")",
            "res": "ContractAccess(\"@1EditLang\")",
            "conditions": "ContractAccess(\"@1EditLang\")",
            "permissions": "ContractConditions(\"@1MainCondition\")",
            "ecosystem": "false"
        }',
        'ContractConditions("@1MainCondition")', '{{.Ecosystem}}'
    ),
    (next_id('1_tables'), 'menu',
        '{
            "insert": "ContractConditions(\"DeveloperCondition\")",
            "update": "ContractConditions(\"DeveloperCondition\")",
            "new_column": "ContractConditions(\"@1MainCondition\")"
        }',
        '{
            "name": "false",
            "value": "ContractAccess(\"@1EditMenu\",\"@1AppendMenu\")",
            "title": "ContractAccess(\"@1EditMenu\")",
            "conditions": "ContractAccess(\"@1EditMenu\")",
            "permissions": "ContractConditions(\"@1MainCondition\")",
            "ecosystem": "false"
        }',
        'ContractConditions("@1MainCondition")', '{{.Ecosystem}}'
    ),
    (next_id('1_tables'), 'pages',
        '{
            "insert": "ContractConditions(\"DeveloperCondition\")",
            "update": "ContractConditions(\"DeveloperCondition\")",
            "new_column": "ContractConditions(\"@1MainCondition\")"
        }',
        '{
            "name": "false",
            "value": "ContractAccess(\"@1EditPage\",\"@1AppendPage\")",
            "menu": "ContractAccess(\"@1EditPage\")",
            "validate_count": "ContractAccess(\"@1EditPage\")",
            "validate_mode": "ContractAccess(\"@1EditPage\")",
            "app_id": "ContractAccess(\"@1ItemChangeAppId\")",
            "conditions": "ContractAccess(\"@1EditPage\")",
            "permissions": "ContractConditions(\"@1MainCondition\")",
            "ecosystem": "false"
        }',
        'ContractConditions("@1MainCondition")', '{{.Ecosystem}}'
    ),
    (next_id('1_tables'), 'snippets',
        '{
            "insert": "ContractConditions(\"DeveloperCondition\")",
            "update": "ContractConditions(\"DeveloperCondition\")",
            "new_column": "ContractConditions(\"@1MainCondition\")"
        }',
        '{
            "name": "false",
            "value": "ContractAccess(\"@1EditSnippet\")",
            "conditions": "ContractAccess(\"@1EditSnippet\")",
            "permissions": "ContractConditions(\"@1MainCondition\")",
            "app_id": "ContractAccess(\"@1ItemChangeAppId\")",
            "ecosystem": "false"
        }',
        'ContractConditions("@1MainCondition")', '{{.Ecosystem}}'
    ),
    (next_id('1_tables'), 'members',
        '{
            "insert": "ContractAccess(\"@1ProfileEdit\")",
            "update": "ContractAccess(\"@1ProfileEdit\")",
            "new_column": "ContractConditions(\"@1MainCondition\")"
        }',
        '{
            "image_id": "ContractAccess(\"@1ProfileEdit\")",
            "member_info": "ContractAccess(\"@1ProfileEdit\")",
            "member_name": "false",
            "account":"false",
            "ecosystem": "false"
        }',
        'ContractConditions("@1MainCondition")', '{{.Ecosystem}}'
    ),
    (next_id('1_tables'), 'roles',
        '{
            "insert": "ContractAccess(\"@1RolesCreate\",\"@1RolesInstall\")",
            "update": "ContractAccess(\"@1RolesAccessManager\",\"@1RolesDelete\")",
            "new_column": "ContractConditions(\"@1MainCondition\")"
        }',
        '{
            "default_page": "false",
            "creator": "false",
            "deleted": "ContractAccess(\"@1RolesDelete\")",
            "company_id": "false",
            "date_deleted": "ContractAccess(\"@1RolesDelete\")",
            "image_id": "false",
            "role_name": "false",
            "date_created": "false",
            "roles_access": "ContractAccess(\"@1RolesAccessManager\")",
            "role_type": "false",
            "ecosystem": "false"
        }',
        'ContractConditions("@1MainCondition")', '{{.Ecosystem}}'
    ),
    (next_id('1_tables'), 'roles_participants',
        '{
            "insert": "ContractAccess(\"@1RolesAssign\",\"@1VotingDecisionCheck\",\"@1RolesInstall\")",
            "update": "ContractAccess(\"@1RolesUnassign\")",
            "new_column": "ContractConditions(\"@1MainCondition\")"
        }',
        '{
            "deleted": "ContractAccess(\"@1RolesUnassign\")",
            "date_deleted": "ContractAccess(\"@1RolesUnassign\")",
            "member": "false",
            "role": "false",
            "date_created": "false",
            "appointed": "false",
            "ecosystem": "false"
        }',
        'ContractConditions("@1MainCondition")', '{{.Ecosystem}}'
    ),
    (next_id('1_tables'), 'notifications',
        '{
            "insert": "ContractAccess(\"@1NotificationsSend\", \"@1CheckNodesBan\", \"@1NotificationsBroadcast\")",
            "update": "ContractAccess(\"@1NotificationsSend\", \"@1NotificationsClose\", \"@1NotificationsProcess\", \"@1NotificationsUpdateParams\")",
            "new_column": "ContractConditions(\"@1MainCondition\")"
        }',
        '{
            "date_closed": "ContractAccess(\"@1NotificationsClose\")",
            "sender": "false",
            "processing_info": "ContractAccess(\"@1NotificationsClose\",\"@1NotificationsProcess\")",
            "date_start_processing": "ContractAccess(\"@1NotificationsClose\",\"@1NotificationsProcess\")",
            "notification": "false",
            "page_name": "false",
            "page_params": "ContractAccess(\"@1NotificationsUpdateParams\")",
            "closed": "ContractAccess(\"@1NotificationsClose\")",
            "date_created": "false",
            "recipient": "false",
            "ecosystem": "false"
        }',
        'ContractConditions("@1MainCondition")', '{{.Ecosystem}}'
    ),
    (next_id('1_tables'), 'sections',
        '{
            "insert": "ContractConditions(\"DeveloperCondition\")",
            "update": "ContractConditions(\"DeveloperCondition\")",
            "new_column": "ContractConditions(\"@1MainCondition\")"
        }',
        '{
            "title": "ContractAccess(\"@1EditSection\")",
            "urlname": "ContractAccess(\"@1EditSection\")",
            "page": "ContractAccess(\"@1EditSection\")",
            "roles_access": "ContractAccess(\"@1SectionRoles\")",
            "status": "ContractAccess(\"@1EditSection\",\"@1NewSection\")",
            "ecosystem": "false"
        }',
        'ContractConditions("@1MainCondition")', '{{.Ecosystem}}'
    ),
    (next_id('1_tables'), 'applications',
        '{
            "insert": "ContractConditions(\"DeveloperCondition\")",
            "update": "ContractConditions(\"DeveloperCondition\")",
            "new_column": "ContractConditions(\"@1MainCondition\")"
        }',
        '{
            "name": "false",
            "uuid": "false",
            "conditions": "ContractAccess(\"@1EditApplication\")",
            "deleted": "ContractAccess(\"@1DelApplication\")",
            "ecosystem": "false"
        }',
        'ContractConditions("@1MainCondition")', '{{.Ecosystem}}'
    ),
    (next_id('1_tables'), 'binaries',
        '{
            "insert": "ContractAccess(\"@1UploadBinary\")",
            "update": "ContractAccess(\"@1UploadBinary\")",
            "new_column": "ContractConditions(\"@1MainCondition\")"
        }',
        '{
            "hash": "ContractAccess(\"@1UploadBinary\")",
            "account": "false",
            "data": "ContractAccess(\"@1UploadBinary\")",
            "name": "false",
            "app_id": "false",
            "ecosystem": "false",
            "mime_type": "ContractAccess(\"@1UploadBinary\")"
        }',
        'ContractConditions("@1MainCondition")', '{{.Ecosystem}}'
    ),
    (next_id('1_tables'), 'parameters',
        '{
            "insert": "ContractConditions(\"DeveloperCondition\")",
            "update": "ContractAccess(\"@1EditParameter\")",
            "new_column": "ContractConditions(\"@1MainCondition\")"
        }',
        '{
            "name": "false",
            "value": "ContractAccess(\"@1EditParameter\")",
            "conditions": "ContractAccess(\"@1EditParameter\")",
            "permissions": "ContractConditions(\"@1MainCondition\")",
            "ecosystem": "false"
        }',
        'ContractConditions("@1MainCondition")', '{{.Ecosystem}}'
    ),
    (next_id('1_tables'), 'app_params',
        '{
            "insert": "ContractConditions(\"DeveloperCondition\")",
            "update": "ContractAccess(\"@1EditAppParam\")",
            "new_column": "ContractConditions(\"@1MainCondition\")"
        }',
        '{
            "app_id": "ContractAccess(\"@1ItemChangeAppId\")",
            "name": "false",
            "value": "ContractAccess(\"@1EditAppParam\")",
            "conditions": "ContractAccess(\"@1EditAppParam\")",
            "permissions": "ContractConditions(\"@1MainCondition\")",
            "ecosystem": "false"
        }',
        'ContractConditions("@1MainCondition")', '{{.Ecosystem}}'
    ),
    (next_id('1_tables'), 'buffer_data',
        '{
            "insert": "true",
            "update": "true",
            "new_column": "ContractConditions(\"@1MainCondition\")"
        }',
        '{
            "key": "false",
            "value": "true",
            "account": "false",
            "ecosystem": "false"
        }',
        'ContractConditions("@1MainCondition")', '{{.Ecosystem}}'
    ),
	(next_id('1_tables'), 'views',
        '{
            "insert": "ContractConditions(\"DeveloperCondition\")",
            "update": "false",
            "new_column": "false"
        }',
        '{
            "name": "false",
            "columns": "false",
            "wheres": "false",
            "permissions": "true",
            "conditions": "true",
            "app_id": "false",
            "ecosystem": "false"
        }',
        'ContractConditions("@1MainCondition")', '{{.Ecosystem}}'
    );
`
