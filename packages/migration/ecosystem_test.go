/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

)

func TestGetEcosystemScript(t *testing.T) {
	str := fmt.Sprintf(GetFirstEcosystemScript(), -1744264011260937456)
	os.WriteFile("/home/losaped/ecosystem_test.sql", []byte(str), 0777)
}
