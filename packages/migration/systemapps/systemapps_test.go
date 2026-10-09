/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package systemapps

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadReadsEveryRecordTheManifestNames(t *testing.T) {
	apps, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "System", apps.manifest.Application)
	records := len(apps.manifest.Pages) + len(apps.manifest.Blocks) + len(apps.manifest.Menus) + len(apps.manifest.Parameters)
	assert.Len(t, apps.sources, records)
	for file, source := range apps.sources {
		assert.True(t, strings.HasPrefix(source, "@wtl 1\n"), file)
	}
	for name, texts := range apps.languages {
		assert.ElementsMatch(t, []string{"en", "zh", "tr"}, keys(texts), name)
	}
}

func keys(texts map[string]string) (found []string) {
	for language := range texts {
		found = append(found, language)
	}
	return
}

func TestFirstEcosystemSQLInsertsEachRecordOnce(t *testing.T) {
	apps, err := Load()
	require.NoError(t, err)
	sql, err := apps.FirstEcosystemSQL()
	require.NoError(t, err)
	for _, table := range []string{"1_pages", "1_snippets", "1_menu", "1_languages"} {
		assert.Equal(t, 1, strings.Count(sql, `INSERT INTO "`+table+`"`), table)
	}
	for _, page := range apps.manifest.Pages {
		assert.Contains(t, sql, "(next_id('1_pages'), '"+page.Name+"', ", page.Name)
	}
	assert.Equal(t, len(apps.languages), strings.Count(sql, "next_id('1_languages')"))
	res, err := json.Marshal(apps.languages["system_home"])
	require.NoError(t, err)
	assert.Contains(t, sql, "'system_home', "+quote(string(res)))
	assert.NotContains(t, sql, "parameters/")
}

func TestPlatformParametersSQLGivesANewEcosystemItsDocuments(t *testing.T) {
	apps, err := Load()
	require.NoError(t, err)
	sql := apps.PlatformParametersSQL()
	assert.Contains(t, sql, "'default_ecosystem_page', '@wtl 1\n<Page>")
	assert.Contains(t, sql, "'default_ecosystem_menu', '@wtl 1\n<Menu />\n', 'ContractAccess(\"@1UpdatePlatformParam\")'")
}

func TestQuoteDoublesQuotesOnly(t *testing.T) {
	assert.Equal(t, `'it''s \n'`, quote(`it's \n`))
	assert.Equal(t, "", insert("1_menu", "id", nil))
}
