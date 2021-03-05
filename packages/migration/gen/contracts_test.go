/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package main

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEscape(t *testing.T) {
	var cases = []struct {
		Source   string

	file.Write([]byte(fmt.Sprintf(`// +prop AppID = %d
// +prop Conditions = '%s'
%s`, appID, conditions, value)))

	return file.Name(), nil
}

func TestLoadSource(t *testing.T) {
	value := "contract Test {}"

	path, err := tempContract(5, "true", value)
	assert.NoError(t, err)

	source, err := loadSource(path)
	assert.NoError(t, err)

	assert.Equal(t, &contract{
		Name:       filepath.Base(path),
		Source:     template.HTML(value + "\n"),
		Conditions: template.HTML("true"),
		AppID:      5,
	}, source)
}
