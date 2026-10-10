/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

// Package systemapps holds the system apps of ecosystem 1 as their one source, weaver-next's
// system-apps/, keeps them: WTL records, language resources and the manifest naming each record.
// Its scripts/genesis.ts writes the files beside this one; they are never edited here. The genesis
// data of ecosystem 1 and the platform parameters giving a new ecosystem its default page and menu
// are made from them.
package systemapps

import (
	"embed"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/IBAX-io/go-ibax/packages/wtl"
)

//go:embed manifest.json languages.json pages blocks menus parameters
var files embed.FS

// The application of ecosystem 1 the records belong to, as the genesis data numbers it
const applicationID = 1

// Who may change a platform parameter, as the genesis data has it for each
const parameterConditions = `ContractAccess("@1UpdatePlatformParam")`

type manifest struct {
	Application string `json:"application"`
	Pages       []struct {
		Name       string `json:"name"`
		Menu       string `json:"menu"`
		Conditions string `json:"conditions"`
	} `json:"pages"`
	Blocks []struct {
		Name       string `json:"name"`
		Conditions string `json:"conditions"`
	} `json:"blocks"`
	Menus []struct {
		Name       string `json:"name"`
		Title      string `json:"title"`
		Conditions string `json:"conditions"`
	} `json:"menus"`
	Parameters []struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
	} `json:"parameters"`
}

// Apps are the system apps read from the files embedded
type Apps struct {
	manifest  manifest
	languages map[string]map[string]string
	sources   map[string]string
}

// Load reads the system apps embedded; an error means the files are not as genesis.ts writes them
func Load() (*Apps, error) {
	apps := &Apps{sources: map[string]string{}}
	if err := readJSON("manifest.json", &apps.manifest); err != nil {
		return nil, err
	}
	if err := readJSON("languages.json", &apps.languages); err != nil {
		return nil, err
	}
	read := func(directory, name string) error {
		file := path.Join(directory, name+".wtl")
		data, err := files.ReadFile(file)
		if err != nil {
			return fmt.Errorf("system apps: %w", err)
		}
		apps.sources[file] = string(data)
		return nil
	}
	for _, page := range apps.manifest.Pages {
		if err := read("pages", page.Name); err != nil {
			return nil, err
		}
	}
	for _, block := range apps.manifest.Blocks {
		if err := read("blocks", block.Name); err != nil {
			return nil, err
		}
	}
	for _, menu := range apps.manifest.Menus {
		if err := read("menus", menu.Name); err != nil {
			return nil, err
		}
	}
	for _, parameter := range apps.manifest.Parameters {
		if err := read("parameters", parameter.Name); err != nil {
			return nil, err
		}
	}
	return apps, nil
}

func readJSON(file string, into any) error {
	data, err := files.ReadFile(file)
	if err != nil {
		return fmt.Errorf("system apps: %w", err)
	}
	if err := json.Unmarshal(data, into); err != nil {
		return fmt.Errorf("system apps: %s: %w", file, err)
	}
	return nil
}

// A string literal of SQL; the node's connections keep standard_conforming_strings, so only the
// quote is doubled
func quote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// The refs of a source of ecosystem 1, as the node writes them with the value (package wtl)
func refs(source string) string {
	return quote(wtl.Read(source, 1).JSON())
}

func insert(table string, columns string, rows []string) string {
	if len(rows) == 0 {
		return ""
	}
	return fmt.Sprintf("INSERT INTO %q (%s) VALUES\n\t%s;\n", table, columns, strings.Join(rows, ",\n\t"))
}

// FirstEcosystemSQL inserts the pages, blocks, menus and language resources of ecosystem 1
func (apps *Apps) FirstEcosystemSQL() (string, error) {
	var pages, blocks, menus, languages []string
	for _, page := range apps.manifest.Pages {
		source := apps.sources[path.Join("pages", page.Name+".wtl")]
		pages = append(pages, fmt.Sprintf("(next_id('1_pages'), %s, %s, %s, %s, '%d', %s, '1')", quote(page.Name),
			quote(source), quote(page.Menu), quote(page.Conditions), applicationID, refs(source)))
	}
	for _, block := range apps.manifest.Blocks {
		source := apps.sources[path.Join("blocks", block.Name+".wtl")]
		blocks = append(blocks, fmt.Sprintf("(next_id('1_snippets'), %s, %s, %s, '%d', %s, '1')", quote(block.Name),
			quote(source), quote(block.Conditions), applicationID, refs(source)))
	}
	for _, menu := range apps.manifest.Menus {
		source := apps.sources[path.Join("menus", menu.Name+".wtl")]
		menus = append(menus, fmt.Sprintf("(next_id('1_menu'), %s, %s, %s, %s, %s, '1')", quote(menu.Name),
			quote(source), quote(menu.Title), quote(menu.Conditions), refs(source)))
	}
	names := make([]string, 0, len(apps.languages))
	for name := range apps.languages {
		names = append(names, name)
	}
	// In the order of their names, so that every node numbers them alike
	slices.Sort(names)
	for _, name := range names {
		res, err := json.Marshal(apps.languages[name])
		if err != nil {
			return "", err
		}
		languages = append(languages, fmt.Sprintf("(next_id('1_languages'), %s, %s, '1')", quote(name), quote(string(res))))
	}
	return insert("1_pages", "id, name, value, menu, conditions, app_id, refs, ecosystem", pages) +
		insert("1_snippets", "id, name, value, conditions, app_id, refs, ecosystem", blocks) +
		insert("1_menu", "id, name, value, title, conditions, refs, ecosystem", menus) +
		insert("1_languages", "id, name, res, ecosystem", languages), nil
}

// PlatformParametersSQL inserts the platform parameters whose value is a document of a new ecosystem
func (apps *Apps) PlatformParametersSQL() string {
	var rows []string
	for _, parameter := range apps.manifest.Parameters {
		rows = append(rows, fmt.Sprintf("(next_id('1_platform_parameters'), %s, %s, %s)", quote(parameter.Name),
			quote(apps.sources[path.Join("parameters", parameter.Name+".wtl")]), quote(parameterConditions)))
	}
	return insert("1_platform_parameters", "id, name, value, conditions", rows)
}
