/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package migration

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"text/template"

	"github.com/gobuffalo/fizz"
	"github.com/gobuffalo/fizz/translators"
)

type SqlData struct {
	Ecosystem   int
	Wallet      int64
	Name        string
	Founder     int64
	AppID       int64
	Account     string
	Digits      int64
	TokenSymbol string
	TokenName   string
}

var _ fizz.Translator = (*translators.Postgres)(nil)
var pgt = translators.NewPostgres()
var tblName string

const (
	sqlPrimary = "primary"
	sqlUnique  = "unique"
	sqlIndex   = "index"
	sqlSeq     = "seq"
)

func sqlHeadSequence(name string) string {
	ret := fmt.Sprintf(`sql("DROP SEQUENCE IF EXISTS %[1]s_id_seq CASCADE;")
sql("CREATE SEQUENCE %[1]s_id_seq START WITH 1;")`, name)

	return ret + "\r\n" + sqlHead(name)
}

func sqlHead(name string) string {
	tblName = name
	return fmt.Sprintf(`sql("DROP TABLE IF EXISTS \"%[1]s\";")
	create_table("%[1]s") {`, name)
}

func sqlEnd(options ...string) (ret string) {
	ret = `t.DisableTimestamps()
	}`
	for _, opt := range options {
		var cname string
		if strings.HasPrefix(opt, sqlSeq) {
			ret += fmt.Sprintf(`
		sql("ALTER SEQUENCE %[1]s_id_seq owned by %[1]s.id;")`, tblName)
			continue
		}
		if strings.HasPrefix(opt, sqlPrimary) {
			if opt == sqlPrimary {
				opt = `PRIMARY KEY (id)`
			} else {
				opt = strings.Replace(opt, sqlPrimary, `PRIMARY KEY `, 1)
			}
			cname = "pkey"
		}
		if strings.HasPrefix(opt, sqlUnique) {
			pars := strings.Split(strings.Trim(opt[len(sqlUnique):], `() `), `,`)
			opt = strings.Replace(opt, sqlUnique, `UNIQUE `, 1)
			for i, val := range pars {
				pars[i] = strings.TrimSpace(val)
			}
			cname = strings.Join(pars, `_`)
		}
		if strings.HasPrefix(opt, sqlIndex) {
			pars := strings.Split(strings.Trim(opt[len(sqlIndex):], `() `), `,`)
			for i, val := range pars {
				pars[i] = strings.TrimSpace(val)
			}
			if len(pars) == 1 {
				ret += fmt.Sprintf(`
		add_index("%s", "%s", {})`, tblName, pars[0])
			} else {
				ret += fmt.Sprintf(`
		add_index("%s", ["%s"], {})`, tblName, strings.Join(pars, `", "`))
			}
			continue
		}
		ret += fmt.Sprintf(`
	sql("ALTER TABLE ONLY \"%[1]s\" ADD CONSTRAINT \"%[1]s_%[3]s\" %[2]s;")`, tblName, opt, cname)
	}
	return
}

func sqlConvert(in []string) (ret string, err error) {
	var item string
	funcs := template.FuncMap{
		"head":    sqlHead,
		"footer":  sqlEnd,
		"headseq": sqlHeadSequence,
	}
	sqlTmpl := template.New("sql").Funcs(funcs)
	for _, sql := range in {
		var (
			tmpl *template.Template
			out  bytes.Buffer
		)

		if tmpl, err = sqlTmpl.Parse(sql); err != nil {
			return
		}
		if err = tmpl.Execute(io.Writer(&out), nil); err != nil {
			return
		}
		item, err = fizz.AString(out.String(), pgt)
		if err != nil {
			return
		}
		ret += item + "\r\n"
	}
	return
}

func sqlTemplate(input []string, data any) (ret string, err error) {
	for _, item := range input {
		var (
			out  bytes.Buffer
			tmpl *template.Template
		)
		tmpl, err = template.New("sql").Parse(item)
		if err != nil {
			return
		}
		if err = tmpl.Execute(io.Writer(&out), data); err != nil {
			return
		}
		ret += out.String() + "\r\n"
	}
	return
}

// GetEcosystemScript returns script to create ecosystem
func GetEcosystemScript(data SqlData) (string, error) {
	return sqlTemplate([]string{
		contractsDataSQL,
		menuDataSQL,
		pagesDataSQL,
		parametersDataSQL,
		membersDataSQL,
		sectionsDataSQL,
		keysDataSQL,
	}, data)
}

// GetFirstEcosystemScript returns script to update with additional data for first ecosystem
func GetFirstEcosystemScript(data SqlData) (ret string, err error) {
	ret, err = sqlConvert([]string{
		sqlFirstEcosystemSchema,
	})
	if err != nil {
		return
	}
	var out string
	out, err = sqlTemplate([]string{
		firstDelayedContractsDataSQL,
		firstEcosystemDataSQL,
	}, data)
	ret += out

	scripts := []string{
		firstEcosystemContractsSQL,
		firstEcosystemPagesDataSQL,
		firstEcosystemBlocksDataSQL,
		platformParametersDataSQL,
		firstTablesDataSQL,
	}
	ret += strings.Join(scripts, "\r\n")
	return
}

// GetFirstTableScript returns script to update _tables for first ecosystem
func GetFirstTableScript(data SqlData) (string, error) {
	return sqlTemplate([]string{
		tablesDataSQL,
	}, data)
}

// GetCommonEcosystemScript returns script with common tables
func GetCommonEcosystemScript() (string, error) {
	sql, err := sqlConvert([]string{
		sqlFirstEcosystemCommon,
		sqlTimeZonesSQL,
	})
	if err != nil {
		return ``, err
	}
	return sql + "\r\n" + timeZonesSQL, nil
}
