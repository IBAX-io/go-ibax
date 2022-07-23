/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package api

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/types"
)

func ImportApps(path, appname string) error {
	apps, err := os.ReadFile(path + "/" + appname + ".json")
	if err != nil {
		return err
	}
	var val = make(map[any]any)
	val["Body"] = apps
	val["MimeType"] = "application/json"
	val["Name"] = appname + ".json"

	params := contractParams{
		"Data": val,
	}
	_, _, err = postTxResult("ImportUpload", &params)
	if err != nil {
		return err
	}
	fmt.Println("successful upload ", val["Name"], "---------------")
	damap, err := smart.JSONDecode(string(apps))
	if err != nil {
		return err
	}
	if _, o := damap.(*types.Map).Get("data"); o {
		vals := converter.MarshalJson(damap.(*types.Map).Values()[1])

		params2 := url.Values{`Data`: {vals}}
		_, _, err = postTxResult("Import", &params2)
		if err != nil {
			return err
		}
		fmt.Println("successful import ", val["Name"], "---------------")
	} else {
		return errors.New("nil data")
	}
	return nil
}
func TestImportApps(t *testing.T) {
	assert.NoError(t, keyLogin(1))
	path, err := os.Getwd()
	assert.NoError(t, err)
	assert.NoError(t, ImportApps(path, "system"))
	assert.NoError(t, ImportApps(path, "conditions"))
	assert.NoError(t, ImportApps(path, "basic"))
	assert.NoError(t, ImportApps(path, "lang_res"))
	assert.NoError(t, ImportApps(path, "platform_apps/ecosystems_catalog"))
	assert.NoError(t, ImportApps(path, "platform_apps/token_emission"))
	form := url.Values{}
	assert.NoError(t, postTx(`@1RolesInstall`, &form))
	fmt.Println("successful RolesInstall ")

	//form = url.Values{"SetDefault": {"yes"}}
	//assert.NoError(t, postTx(`@1VotingTemplatesInstall`, &form))
	//fmt.Println("successful VotingTemplatesInstall ")
	//nodePub := `0498b18e551493a269b6f419d7784d26c8e3555638e80897c69997ef9f211e21d5d0b8adeeaab0e0e750e720ddf3048ec55d613ba5dee3fdfd4e7c17d346731e9b`
	//tcpHost := `127.0.0.1:7078`
	//firstNode := fmt.Sprintf(`{"api_address":"%v","public_key":"%v","tcp_address":"%v"}`, apiAddress, nodePub, tcpHost)
	//firstNodeID := `18`
	//form = url.Values{"Conditions": {`ContractConditions("@1DeveloperCondition")`},
	//	"Id":    {firstNodeID},
	//	"Value": {firstNode},
	//}
	//assert.NoError(t, postTx(`@1EditAppParam`, &form))
	//fmt.Println("successful EditAppParam to first_node ")
	//users := []string{
	//	`04794cbbfa0ff0d1a3dc3e08e5332ff44131be265d9d67ad60996fd5e3f04d50610b8de6b99bb068991a29806e16832290c0bc890373ae592037317fa213227e39`,
	//	`045b9c7555a9218f67a94c54c740e33bac6658d6f19be3d527932ccf067cecea17d9d104c315f603458c4ff022af6234e6ee2c8772d334c6a4f478c25fd5ac9a81`,
	//	`0432ed8601fbe0e452f647147e26bfbfc93532e019f9dad80d183c3dafe7d432f9d84cff6595d9c023fc40119f0b31fa3b2e05d6511bb83f1ba38eb487df8cafe1`,
	//	`046ea9
	//
	////	254d1d7a530794ef7f5798dcd7842829628ab3901f72b8ae3944d77403bdb9ac4cd286c664d7f247f83b88844bf4c1d3cc8990cd3944db49cf494cb277b3`,
	//}
	//for _, u := range users {
	//	form = url.Values{"NewPubkey": {u}}
	//	assert.NoError(t, postTx(`@1NewUser`, &form))
	//}
	//fmt.Println("successful 4 NewUser ")
	//form = url.Values{
	//	"TcpAddress":  {`127.0.0.1:8078`},
	//	"ApiAddress":  {`http://127.0.0.1:8079`},
	//	"PubKey":      {`04794cbbfa0ff0d1a3dc3e08e5332ff44131be265d9d67ad60996fd5e3f04d50610b8de6b99bb068991a29806e16832290c0bc890373ae592037317fa213227e39`},
	//	"Description": {`node2`}}
	//assert.NoError(t, postTx(`@1CNConnectionRequest`, &form))
	//fmt.Println("successful node2 CNConnectionRequest ")
	//form = url.Values{
	//	"TcpAddress":  {`127.0.0.1:9078`},
	//	"ApiAddress":  {`http://127.0.0.1:9079`},
	//	"PubKey":      {`0432ed8601fbe0e452f647147e26bfbfc93532e019f9dad80d183c3dafe7d432f9d84cff6595d9c023fc40119f0b31fa3b2e05d6511bb83f1ba38eb487df8cafe1`},
	//	"Description": {`node3`}}
	//assert.NoError(t, postTx(`@1CNConnectionRequest`, &form))
	//fmt.Println("successful node3 CNConnectionRequest ")
}
