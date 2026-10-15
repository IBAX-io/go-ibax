/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// ecosystemsAccess returns the permissions and columns the first ecosystem script gives the
// ecosystems table.
func ecosystemsAccess(t *testing.T, data SqlData) (permissions, columns map[string]string) {
	t.Helper()
	script, err := GetFirstEcosystemScript(data)
	if err != nil {
		t.Fatal(err)
	}
	row := script[strings.Index(script, `'ecosystems',`):]
	parts := strings.Split(row, `'`)
	if err := json.Unmarshal([]byte(parts[3]), &permissions); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(parts[5]), &columns); err != nil {
		t.Fatal(err)
	}
	return
}

func TestFirstTablesEcosystemsAccess(t *testing.T) {
	access := func(contracts ...string) string {
		return `ContractAccess("@1` + strings.Join(contracts, `","@1`) + `")`
	}
	update := []string{"EditEcosystemName", "VotingVesAccept", "EcManageInfo", "EcoFeeModeManage",
		"EditControlMode", "NewToken", "TeChange"}
	common := map[string]string{
		"name":          access("EditEcosystemName"),
		"info":          access("EcManageInfo"),
		"fee_mode_info": access("EcoFeeModeManage"),
		"is_valued":     access("VotingVesAccept"),
		"token_symbol":  access("NewToken"),
		"token_name":    access("NewToken"),
		"control_mode":  access("EditControlMode"),
	}
	columns := func(extra map[string]string) map[string]string {
		m := map[string]string{}
		for k, v := range common {
			m[k] = v
		}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	for _, c := range []struct {
		name    string
		data    SqlData
		update  string
		columns map[string]string
	}{
		{"before #2072", SqlData{}, access(append(update, "TeBurn")...), columns(map[string]string{
			"emission_amount": access("NewToken", "TeBurn"),
			"type_emission":   access("NewToken", "TeChange"),
			"type_withdraw":   access("NewToken", "TeChange"),
		})},
		{"#2072", SqlData{TeEmissionAccess: true}, access(append(update, "TeEmission", "TeBurn")...), columns(map[string]string{
			"emission_amount": access("NewToken", "TeBurn", "TeEmission"),
			"type_emission":   access("TeChange"),
			"type_withdraw":   access("TeChange"),
		})},
		{"#2133", SqlData{TeEmissionAccess: true, DigitsAccess: true}, access(append(update, "TeEmission", "TeBurn")...), columns(map[string]string{
			"digits":          access("NewToken"),
			"emission_amount": access("NewToken", "TeBurn", "TeEmission"),
			"type_emission":   access("TeChange", "NewToken"),
			"type_withdraw":   access("TeChange", "NewToken"),
		})},
	} {
		permissions, cols := ecosystemsAccess(t, c.data)
		if permissions["update"] != c.update {
			t.Errorf("%s: update %s, want %s", c.name, permissions["update"], c.update)
		}
		if !reflect.DeepEqual(cols, c.columns) {
			t.Errorf("%s: columns %v, want %v", c.name, cols, c.columns)
		}
	}
}
