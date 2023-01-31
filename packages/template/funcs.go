/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package template

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/IBAX-io/go-ibax/packages/common/crypto"
	"github.com/IBAX-io/go-ibax/packages/conf/syspar"
	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/language"
	"github.com/IBAX-io/go-ibax/packages/smart"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"
	"github.com/IBAX-io/go-ibax/packages/types"
	"github.com/IBAX-io/go-ibax/packages/utils"

	qb "github.com/IBAX-io/go-ibax/packages/storage/sqldb/queryBuilder"

	log "github.com/sirupsen/logrus"
)

// Composite represents a composite contract
type Composite struct {
	Name string `json:"name"`
	Data any    `json:"data,omitempty"`
}

// Action describes a button action
type Action struct {
	Name   string            `json:"name"`
	Params map[string]string `json:"params,omitempty"`
}

var (
	funcs = make(map[string]tplFunc)
	tails = make(map[string]forTails)
	modes = [][]rune{{'(', ')'}, {'{', '}'}, {'[', ']'}}
)

const (
	columnNameKey = "column_name"
	dataTypeKey   = "data_type"
)

func init() {
	funcs[`Lower`] = tplFunc{lowerTag, defaultTag, `lower`, `Text`}
	funcs[`AddToolButton`] = tplFunc{defaultTailTag, defaultTailTag, `addtoolbutton`, `Title,Icon,Page,PageParams`}
	funcs[`Address`] = tplFunc{addressTag, defaultTag, `address`, `Wallet`}
	funcs[`PubToID`] = tplFunc{pubToIdTag, defaultTag, `pubtoid`, `Pub`}
	funcs[`AddressToId`] = tplFunc{addressIDTag, defaultTag, `addresstoid`, `Wallet`}
	funcs[`AppParam`] = tplFunc{appparTag, defaultTag, `apppar`, `Name,App,Index,Source,Ecosystem`}
	funcs[`Calculate`] = tplFunc{calculateTag, defaultTag, `calculate`, `Exp,Type,Prec`}
	funcs[`CmpTime`] = tplFunc{cmpTimeTag, defaultTag, `cmptime`, `Time1,Time2`}
	funcs[`Code`] = tplFunc{defaultTag, defaultTag, `code`, `Text`}
	funcs[`CodeAsIs`] = tplFunc{defaultTag, defaultTag, `code`, `#Text`}
	funcs[`DateTime`] = tplFunc{dateTimeTag, defaultTag, `datetime`, `DateTime,Format,Location`}
	funcs[`EcosysParam`] = tplFunc{ecosysparTag, defaultTag, `ecosyspar`, `Name,Index,Source,Ecosystem`}
	funcs[`Em`] = tplFunc{defaultTag, defaultTag, `em`, `Body,Class`}
	funcs[`GetVar`] = tplFunc{getvarTag, defaultTag, `getvar`, `Name`}
	funcs[`GetHistory`] = tplFunc{getHistoryTag, defaultTag, `gethistory`,
		`Source,Name,Id,RollbackId`}
	funcs[`Hint`] = tplFunc{defaultTag, defaultTag, `hint`, `Icon,Title,Text`}
	funcs[`ImageInput`] = tplFunc{defaultTag, defaultTag, `imageinput`, `Name,Width,Ratio,Format`}
	funcs[`InputErr`] = tplFunc{defaultTag, defaultTag, `inputerr`, `*`}
	funcs[`JsonToSource`] = tplFunc{jsontosourceTag, defaultTag, `jsontosource`, `Source,Data,Prefix`}
	funcs[`ArrayToSource`] = tplFunc{arraytosourceTag, defaultTag, `arraytosource`, `Source,Data,Prefix`}
	funcs[`LangRes`] = tplFunc{langresTag, defaultTag, `langres`, `Name,Lang`}
	funcs[`MenuGroup`] = tplFunc{menugroupTag, defaultTag, `menugroup`, `Title,Body,Icon`}
	funcs[`MenuItem`] = tplFunc{defaultTag, defaultTag, `menuitem`, `Title,Page,PageParams,Icon,Clb`}
	funcs[`Money`] = tplFunc{moneyTag, defaultTag, `money`, `Exp,Digit`}
	funcs[`Range`] = tplFunc{rangeTag, defaultTag, `range`, `Source,From,To,Step`}
	funcs[`SetTitle`] = tplFunc{defaultTag, defaultTag, `settitle`, `Title`}
	funcs[`SetVar`] = tplFunc{setvarTag, defaultTag, `setvar`, `Name,Value`}
	funcs[`Strong`] = tplFunc{defaultTag, defaultTag, `strong`, `Body,Class`}
	funcs[`SysParam`] = tplFunc{sysparTag, defaultTag, `syspar`, `Name`}
	funcs[`Button`] = tplFunc{buttonTag, buttonTag, `button`, `Body,Page,Class,Contract,Params,PageParams`}
	funcs[`Div`] = tplFunc{defaultTailTag, defaultTailTag, `div`, `Class,Body`}
	funcs[`ForList`] = tplFunc{forlistTag, defaultTag, `forlist`, `Source,Data,Index`}
	funcs[`Form`] = tplFunc{defaultTailTag, defaultTailTag, `form`, `Class,Body`}
	funcs[`If`] = tplFunc{ifTag, ifFull, `if`, `Condition,Body`}
	funcs[`Image`] = tplFunc{imageTag, defaultTailTag, `image`, `Src,Alt,Class`}
	funcs[`Include`] = tplFunc{includeTag, defaultTag, `include`, `Name`}
	funcs[`Input`] = tplFunc{defaultTailTag, defaultTailTag, `input`, `Name,Class,Placeholder,Type,Value,Disabled`}
	funcs[`Label`] = tplFunc{defaultTailTag, defaultTailTag, `label`, `Body,Class,For`}
	funcs[`LinkPage`] = tplFunc{defaultTailTag, defaultTailTag, `linkpage`, `Body,Page,Class,PageParams`}
	funcs[`Data`] = tplFunc{dataTag, defaultTailTag, `data`, `Source,Columns,Data`}
	funcs[`DBFind`] = tplFunc{dbfindTag, defaultTailTag, `dbfind`, `Name,Source`}
	funcs[`And`] = tplFunc{andTag, defaultTag, `and`, `*`}
	funcs[`Or`] = tplFunc{orTag, defaultTag, `or`, `*`}
	funcs[`P`] = tplFunc{defaultTailTag, defaultTailTag, `p`, `Body,Class`}
	funcs[`RadioGroup`] = tplFunc{defaultTailTag, defaultTailTag, `radiogroup`, `Name,Source,NameColumn,ValueColumn,Value,Class`}
	funcs[`Span`] = tplFunc{defaultTailTag, defaultTailTag, `span`, `Body,Class`}
	funcs[`QRcode`] = tplFunc{defaultTag, defaultTag, `qrcode`, `Text`}
	funcs[`Table`] = tplFunc{tableTag, defaultTailTag, `table`, `Source,Columns`}
	funcs[`Select`] = tplFunc{defaultTailTag, defaultTailTag, `select`, `Name,Source,NameColumn,ValueColumn,Value,Class`}
	funcs[`Chart`] = tplFunc{chartTag, defaultTailTag, `chart`, `Type,Source,FieldLabel,FieldValue,Colors`}
	funcs[`InputMap`] = tplFunc{defaultTailTag, defaultTailTag, "inputMap", "Name,@Value,Type,MapType"}
	funcs[`Map`] = tplFunc{defaultTag, defaultTag, "map", "@Value,MapType,Hmap"}
	funcs[`Binary`] = tplFunc{binaryTag, defaultTag, "binary", "AppID,Name,Account"}
	funcs[`GetColumnType`] = tplFunc{columntypeTag, defaultTag, `columntype`, `Table,Column`}
	funcs[`VarAsIs`] = tplFunc{varasisTag, defaultTag, `varasis`, `Name,Value`}

	tails[`addtoolbutton`] = forTails{map[string]tailInfo{
		`Popup`: {tplFunc{popupTag, defaultTailFull, `popup`, `Width,Header`}, true},
	}}
	tails[`button`] = forTails{map[string]tailInfo{
		`Action`:            {tplFunc{actionTag, defaultTailFull, `action`, `Name,Params`}, false},
		`Alert`:             {tplFunc{alertTag, defaultTailFull, `alert`, `Text,ConfirmButton,CancelButton,Icon`}, true},
		`Popup`:             {tplFunc{popupTag, defaultTailFull, `popup`, `Width,Header`}, true},
		`Style`:             {tplFunc{tailTag, defaultTailFull, `style`, `Style`}, false},
		`CompositeContract`: {tplFunc{compositeTag, defaultTailFull, `composite`, `Name,Data`}, false},
		`ErrorRedirect`: {tplFunc{errredirTag, defaultTailFull, `errorredirect`,
			`ErrorID,PageName,PageParams`}, false},
	}}
	tails[`div`] = forTails{map[string]tailInfo{
		`Style`: {tplFunc{tailTag, defaultTailFull, `style`, `Style`}, false},
		`Show`:  {tplFunc{showTag, defaultTailFull, `show`, `Condition`}, false},
		`Hide`:  {tplFunc{hideTag, defaultTailFull, `hide`, `Condition`}, false},
	}}
	tails[`form`] = forTails{map[string]tailInfo{
		`Style`: {tplFunc{tailTag, defaultTailFull, `style`, `Style`}, false},
	}}
	tails[`if`] = forTails{map[string]tailInfo{
		`Else`:   {tplFunc{elseTag, elseFull, `else`, `Body`}, true},
		`ElseIf`: {tplFunc{elseifTag, elseifFull, `elseif`, `Condition,Body`}, false},
	}}
	tails[`image`] = forTails{map[string]tailInfo{
		`Style`: {tplFunc{tailTag, defaultTailFull, `style`, `Style`}, false},
	}}
	tails[`input`] = forTails{map[string]tailInfo{
		`Validate`: {tplFunc{validateTag, validateFull, `validate`, `*`}, false},
		`Style`:    {tplFunc{tailTag, defaultTailFull, `style`, `Style`}, false},
	}}
	tails[`label`] = forTails{map[string]tailInfo{
		`Style`: {tplFunc{tailTag, defaultTailFull, `style`, `Style`}, false},
	}}
	tails[`linkpage`] = forTails{map[string]tailInfo{
		`Style`: {tplFunc{tailTag, defaultTailFull, `style`, `Style`}, false},
	}}
	tails[`data`] = forTails{map[string]tailInfo{
		`Custom`: {tplFunc{customTag, customTagFull, `custom`, `Column,Body`}, false},
	}}
	tails[`dbfind`] = forTails{map[string]tailInfo{
		`Columns`: {tplFunc{tailTag, defaultTailFull, `columns`, `Columns`}, false},
		`Count`:   {tplFunc{tailTag, defaultTailFull, `count`, `CountVar`}, false},
		`Where`:   {tplFunc{tailTag, defaultTailFull, `where`, `Where`}, false},
		`WhereId`: {tplFunc{tailTag, defaultTailFull, `whereid`, `WhereId`}, false},
		`Order`:   {tplFunc{tailTag, defaultTailFull, `order`, `Order`}, false},
		`Limit`:   {tplFunc{tailTag, defaultTailFull, `limit`, `Limit`}, false},
		`Offset`:  {tplFunc{tailTag, defaultTailFull, `offset`, `Offset`}, false},
		`Custom`:  {tplFunc{customTag, customTagFull, `custom`, `Column,Body`}, false},
		`Vars`:    {tplFunc{tailTag, defaultTailFull, `vars`, `Prefix`}, false},
		`Cutoff`:  {tplFunc{tailTag, defaultTailFull, `cutoff`, `Cutoff`}, false},
	}}
	tails[`p`] = forTails{map[string]tailInfo{
		`Style`: {tplFunc{tailTag, defaultTailFull, `style`, `Style`}, false},
	}}
	tails[`radiogroup`] = forTails{map[string]tailInfo{
		`Validate`: {tplFunc{validateTag, validateFull, `validate`, `*`}, false},
		`Style`:    {tplFunc{tailTag, defaultTailFull, `style`, `Style`}, false},
	}}
	tails[`span`] = forTails{map[string]tailInfo{
		`Style`: {tplFunc{tailTag, defaultTailFull, `style`, `Style`}, false},
	}}
	tails[`table`] = forTails{map[string]tailInfo{
		`Style`: {tplFunc{tailTag, defaultTailFull, `style`, `Style`}, false},
	}}
	tails[`select`] = forTails{map[string]tailInfo{
		`Validate`: {tplFunc{validateTag, validateFull, `validate`, `*`}, false},
		`Style`:    {tplFunc{tailTag, defaultTailFull, `style`, `Style`}, false},
	}}
	tails[`inputMap`] = forTails{map[string]tailInfo{
		`Validate`: {tplFunc{validateTag, validateFull, `validate`, `*`}, false},
	}}
	tails[`binary`] = forTails{map[string]tailInfo{
		`ById`:      {tplFunc{tailTag, defaultTailFull, `id`, `id`}, false},
		`Ecosystem`: {tplFunc{tailTag, defaultTailFull, `ecosystem`, `ecosystem`}, false},
	}}
}

func defaultTag(par parFunc) string {
	setAllAttr(par)
	par.Owner.Children = append(par.Owner.Children, par.Node)
	return ``
}

func lowerTag(par parFunc) string {
	return strings.ToLower(macro((*par.Pars)[`Text`], par.Workspace.Vars))
}

func moneyTag(par parFunc) string {
	var cents int64
	if len((*par.Pars)[`Digit`]) > 0 {
		cents = converter.StrToInt64(macro((*par.Pars)[`Digit`], par.Workspace.Vars))
	} else {
		ecosystem := getVar(par.Workspace, `ecosystem_id`)
		sp := &sqldb.Ecosystem{}
		_, err := sp.Get(nil, converter.StrToInt64(ecosystem))
		if err != nil {
			return `0`
		}
		cents = sp.Digits
	}
	exp := macro((*par.Pars)[`Exp`], par.Workspace.Vars)
	m, err := converter.FormatMoney(exp, int32(cents))
	if err != nil {
		return `0`
	}
	return m
}

func menugroupTag(par parFunc) string {
	setAllAttr(par)
	name := (*par.Pars)[`Title`]
	if par.RawPars != nil {
		if v, ok := (*par.RawPars)[`Title`]; ok {
			name = v
		}
	}
	par.Node.Attr[`name`] = name
	par.Owner.Children = append(par.Owner.Children, par.Node)
	return ``
}

func forlistTag(par parFunc) (ret string) {
	var (
		name, indexName string
	)
	setAllAttr(par)
	if len((*par.Pars)[`Source`]) > 0 {
		name = par.Node.Attr[`source`].(string)
	}
	if len((*par.Pars)[`Index`]) > 0 {
		indexName = par.Node.Attr[`index`].(string)
	} else {
		indexName = name + `_index`
	}
	if len(name) == 0 || par.Workspace.Sources == nil {
		return
	}
	source := (*par.Workspace.Sources)[name]
	if source.Data == nil {
		return
	}
	root := node{}
	keys := make(map[string]bool)
	for key := range *par.Workspace.Vars {
		keys[key] = true
	}
	for index, item := range *source.Data {
		vals := map[string]string{indexName: converter.IntToStr(index + 1)}
		for i, icol := range *source.Columns {
			vals[icol] = item[i]
		}
		if index > 0 {
			for key := range *par.Workspace.Vars {
				if !keys[key] {
					delete(*par.Workspace.Vars, key)
				}
			}
		}
		for key, item := range vals {
			setVar(par.Workspace, key, item)
		}
		process((*par.Pars)[`Data`], &root, par.Workspace)
		for _, item := range root.Children {
			if item.Tag == `text` {
				item.Text = macroReplace(item.Text, par.Workspace.Vars)
			}
		}
		for key := range vals {
			delete(*par.Workspace.Vars, key)
		}
	}
	par.Node.Children = root.Children
	par.Owner.Children = append(par.Owner.Children, par.Node)
	return
}

func addressTag(par parFunc) string {
	idval := (*par.Pars)[`Wallet`]
	if len(idval) == 0 {
		idval = getVar(par.Workspace, `key_id`)
	}
	idval = processToText(par, macro(idval, par.Workspace.Vars))
	id, _ := strconv.ParseInt(idval, 10, 64)
	if id == 0 {
		return `unknown address`
	}
	return converter.AddressToString(id)
}

func pubToIdTag(par parFunc) string {
	hexkey := (*par.Pars)[`Pub`]
	if len(hexkey) == 0 {
		return `0`
	}
	idval := processToText(par, macro(hexkey, par.Workspace.Vars))
	pubkey, err := crypto.HexToPub(idval)
	if err != nil {
		return `0`
	}
	return converter.Int64ToStr(crypto.Address(pubkey))
}

func addressIDTag(par parFunc) string {
	address := (*par.Pars)[`Wallet`]
	if len(address) == 0 {
		return getVar(par.Workspace, `key_id`)
	}
	id := converter.AddressToID(processToText(par, macro(address, par.Workspace.Vars)))
	if id == 0 {
		return `0`
	}
	return converter.Int64ToStr(id)
}

func calculateTag(par parFunc) string {
	return calculate(macro((*par.Pars)[`Exp`], par.Workspace.Vars), (*par.Pars)[`Type`],
		macro((*par.Pars)[`Prec`], par.Workspace.Vars))
}

func paramToSource(par parFunc, val string) string {
	data := make([][]string, 0)
	cols := []string{`id`, `name`}
	types := []string{`text`, `text`}
	for key, item := range strings.Split(val, `,`) {
		item, _ = language.LangText(nil, item,
			converter.StrToInt(getVar(par.Workspace, `ecosystem_id`)), getVar(par.Workspace, `lang`))
		data = append(data, []string{converter.IntToStr(key + 1), item})
	}
	node := node{Tag: `data`, Attr: map[string]any{`columns`: &cols, `types`: &types,
		`data`: &data, `source`: (*par.Pars)[`Source`]}}
	par.Owner.Children = append(par.Owner.Children, &node)

	par.Workspace.SetSource((*par.Pars)[`Source`], &Source{
		Columns: node.Attr[`columns`].(*[]string),
		Data:    node.Attr[`data`].(*[][]string),
	})

	return ``
}

func paramToIndex(par parFunc, val string) (ret string) {
	ind := converter.StrToInt(macro((*par.Pars)[`Index`], par.Workspace.Vars))
	if alist := strings.Split(val, `,`); ind > 0 && len(alist) >= ind {
		ret, _ = language.LangText(nil, alist[ind-1],
			converter.StrToInt(getVar(par.Workspace, `ecosystem_id`)),
			getVar(par.Workspace, `lang`))
	}
	return
}

func ecosysparTag(par parFunc) string {
	if len((*par.Pars)[`Name`]) == 0 {
		return ``
	}
	ecosystem := getVar(par.Workspace, `ecosystem_id`)
	if len((*par.Pars)[`Ecosystem`]) != 0 {
		ecosystem = macro((*par.Pars)[`Ecosystem`], par.Workspace.Vars)
	}
	sp := &sqldb.StateParameter{}
	sp.SetTablePrefix(ecosystem)
	parameterName := macro((*par.Pars)[`Name`], par.Workspace.Vars)
	_, err := sp.Get(nil, parameterName)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting ecosystem param")
		return err.Error()
	}
	val := sp.Value
	if len((*par.Pars)[`Source`]) > 0 {
		return paramToSource(par, val)
	}
	if len((*par.Pars)[`Index`]) > 0 {
		val = paramToIndex(par, val)
	}
	return val
}

func appparTag(par parFunc) string {
	if len((*par.Pars)[`Name`]) == 0 || len((*par.Pars)[`App`]) == 0 {
		return ``
	}
	ecosystem := getVar(par.Workspace, `ecosystem_id`)
	if len((*par.Pars)[`Ecosystem`]) != 0 {
		ecosystem = macro((*par.Pars)[`Ecosystem`], par.Workspace.Vars)
	}
	ap := &sqldb.AppParam{}
	ap.SetTablePrefix(ecosystem)
	_, err := ap.Get(nil, converter.StrToInt64(macro((*par.Pars)[`App`], par.Workspace.Vars)),
		macro((*par.Pars)[`Name`], par.Workspace.Vars))
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting app param")
		return err.Error()
	}
	val := ap.Value
	if len((*par.Pars)[`Source`]) > 0 {
		return paramToSource(par, val)
	}
	if len((*par.Pars)[`Index`]) > 0 {
		val = paramToIndex(par, val)
	}
	return val
}

func langresTag(par parFunc) string {
	lang := (*par.Pars)[`Lang`]
	if len(lang) == 0 {
		lang = getVar(par.Workspace, `lang`)
	}
	ret, _ := language.LangText(nil, (*par.Pars)[`Name`],
		int(converter.StrToInt64(getVar(par.Workspace, `ecosystem_id`))), lang)
	return ret
}

func sysparTag(par parFunc) (ret string) {
	if len((*par.Pars)[`Name`]) > 0 {
		ret = syspar.SysString(macro((*par.Pars)[`Name`], par.Workspace.Vars))
	}
	return
}

func andTag(par parFunc) string {
	count := len(*par.Pars)
	for i := 0; i < count; i++ {
		if !ifValue((*par.Pars)[strconv.Itoa(i)], par.Workspace) {
			return `0`
		}
	}
	return `1`
}

func orTag(par parFunc) string {
	count := len(*par.Pars)
	for i := 0; i < count; i++ {
		if ifValue((*par.Pars)[strconv.Itoa(i)], par.Workspace) {
			return `1`
		}
	}
	return `0`
}

func alertTag(par parFunc) string {
	setAllAttr(par)
	par.Owner.Attr[`alert`] = par.Node.Attr
	return ``
}

func actionTag(par parFunc) string {
	setAllAttr(par)
	if len((*par.Pars)[`Name`]) == 0 {
		return ``
	}
	if par.Owner.Attr[`action`] == nil {
		par.Owner.Attr[`action`] = make([]Action, 0)
	}
	var params map[string]string
	if v, ok := par.Node.Attr["params"]; ok {
		params = make(map[string]string)
		for key, val := range v.(map[string]any) {
			if imap, ok := val.(map[string]any); ok {
				params[key] = macro(fmt.Sprint(imap["text"]), par.Workspace.Vars)
			} else {
				params[key] = macro(fmt.Sprint(val), par.Workspace.Vars)
			}
		}
	}
	par.Owner.Attr[`action`] = append(par.Owner.Attr[`action`].([]Action),
		Action{
			Name:   macro((*par.Pars)[`Name`], par.Workspace.Vars),
			Params: params,
		})
	return ``
}

func defaultTailFull(par parFunc) string {
	setAllAttr(par)
	par.Owner.Tail = append(par.Owner.Tail, par.Node)
	return ``
}

func dataTag(par parFunc) string {
	setAllAttr(par)
	defaultTail(par, `data`)

	data := make([][]string, 0)
	cols := strings.Split((*par.Pars)[`Columns`], `,`)
	types := make([]string, len(cols))
	for i := 0; i < len(types); i++ {
		types[i] = `text`
	}

	list, err := csv.NewReader(strings.NewReader((*par.Pars)[`Data`])).ReadAll()
	if err != nil {
		input := strings.Split((*par.Pars)[`Data`], "\n")
		par.Node.Attr[`error`] = err.Error()
		prefix := `line `
		for err != nil && strings.HasPrefix(err.Error(), prefix) {
			errText := err.Error()
			line := converter.StrToInt64(errText[len(prefix):strings.IndexByte(errText, ',')])
			if line < 1 {
				break
			}
			input = append(input[:line-1], input[line:]...)
			list, err = csv.NewReader(strings.NewReader(strings.Join(input, "\n"))).ReadAll()
		}
	}
	lencol := 0
	defcol := 0
	for _, item := range list {
		if lencol == 0 {
			defcol = len(cols)
			if par.Node.Attr[`customs`] != nil {
				for _, v := range par.Node.Attr[`customs`].([]string) {
					cols = append(cols, v)
					types = append(types, `tags`)
				}
			}
			lencol = len(cols)
		}
		row := make([]string, lencol)
		vals := make(map[string]Var)
		for i, icol := range cols {
			var ival string
			if i < defcol {
				if i < len(item) {
					ival = strings.TrimSpace(item[i])
				}
				vals[icol] = Var{Value: ival}
			} else {
				root := node{}
				for key, item := range vals {
					(*par.Workspace.Vars)[key] = item
				}
				process(par.Node.Attr[`custombody`].([]string)[i-defcol], &root, par.Workspace)
				for key := range vals {
					delete(*par.Workspace.Vars, key)
				}
				out, err := json.Marshal(root.Children)
				if err == nil {
					ival = macro(string(out), &vals)
				} else {
					log.WithFields(log.Fields{"type": consts.JSONMarshallError, "error": err}).Error("marshalling custombody to JSON")
				}
			}
			row[i] = ival
		}
		data = append(data, row)
	}
	setAllAttr(par)
	delete(par.Node.Attr, `customs`)
	delete(par.Node.Attr, `custombody`)
	par.Node.Attr[`columns`] = &cols
	par.Node.Attr[`types`] = &types
	par.Node.Attr[`data`] = &data
	newSource(par)
	par.Owner.Children = append(par.Owner.Children, par.Node)
	return ``
}

func dbfindTag(par parFunc) string {
	var (
		inColumns any
		columns   []string
		state     int64
		err       error
		perm      map[string]string
		offset    string

		cutoffColumns   = make(map[string]bool)
		extendedColumns = make(map[string]string)
		queryColumns    = make([]string, 0)
	)
	if len((*par.Pars)[`Name`]) == 0 {
		return ``
	}
	defaultTail(par, `dbfind`)
	prefix := ``
	where := ``
	order := ``
	limit := 25

	if par.Node.Attr[`columns`] != nil {
		fields := par.Node.Attr[`columns`].(string)
		if strings.HasPrefix(fields, `[`) {
			inColumns, _, err = ParseObject([]rune(fields))
			if err != nil {
				return err.Error()
			}
		} else {
			inColumns = fields
		}
	}
	columns, err = qb.GetColumns(inColumns)
	if err != nil {
		return err.Error()
	}
	if par.Node.Attr[`where`] != nil {
		where = macro(par.Node.Attr[`where`].(string), par.Workspace.Vars)
		if strings.HasPrefix(where, `{`) {
			inWhere, _, err := ParseObject([]rune(where))
			if err != nil {
				return err.Error()
			}
			switch v := inWhere.(type) {
			case string:
				if len(v) == 0 {
					where = `true`
				} else {
					return errWhere.Error()
				}
			case map[string]any:
				where, err = qb.GetWhere(types.LoadMap(v))
				if err != nil {
					return err.Error()
				}
			case *types.Map:
				where, err = qb.GetWhere(v)
				if err != nil {
					return err.Error()
				}
			default:
				return errWhere.Error()
			}
		} else if len(where) > 0 {
			return errWhere.Error()
		}
	}
	if par.Node.Attr[`whereid`] != nil {
		where = fmt.Sprintf(` id='%d'`, converter.StrToInt64(macro(par.Node.Attr[`whereid`].(string), par.Workspace.Vars)))
	}
	if par.Node.Attr[`limit`] != nil {
		limit = converter.StrToInt(par.Node.Attr[`limit`].(string))
	}
	if limit > consts.DBFindLimit {
		limit = consts.DBFindLimit
	}
	if par.Node.Attr[`offset`] != nil {
		offset = fmt.Sprintf(` offset %d`, converter.StrToInt(par.Node.Attr[`offset`].(string)))
	}

	if par.Node.Attr[`prefix`] != nil {
		prefix = par.Node.Attr[`prefix`].(string)
		limit = 1
	}
	state = converter.StrToInt64(getVar(par.Workspace, `ecosystem_id`))
	if par.Node.Attr["cutoff"] != nil {
		for _, v := range strings.Split(par.Node.Attr["cutoff"].(string), ",") {
			cutoffColumns[v] = true
		}
	}

	sc := par.Workspace.SmartContract
	tblname := converter.ParseTable(strings.Trim(macro((*par.Pars)[`Name`], par.Workspace.Vars), `"`), state)
	tblname = strings.ToLower(tblname)

	inColumns = ``
	if par.Node.Attr[`order`] != nil {
		order = macro(par.Node.Attr[`order`].(string), par.Workspace.Vars)
		if strings.HasPrefix(order, `[`) || strings.HasPrefix(order, `{`) {
			inColumns, _, err = ParseObject([]rune(order))
			if err != nil {
				return err.Error()
			}
		} else {
			inColumns = order
		}
	}
	order, err = qb.GetOrder(tblname, inColumns)
	if err != nil {
		return err.Error()
	}
	order = ` order by ` + order
	rows, err := sc.DbTransaction.GetAllColumnTypes(tblname)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting column types from db")
		return err.Error()
	}
	columnTypes := make(map[string]string, len(rows))
	for _, row := range rows {
		columnTypes[row[columnNameKey]] = row[dataTypeKey]
	}
	columnNames := make([]string, 0)

	perm, err = sc.AccessTablePerm(tblname, `read`)
	if err != nil || sc.AccessColumns(tblname, &columns, false) != nil {
		log.WithFields(log.Fields{"table": tblname, "columns": columns}).Error("ACCESS DENIED")
		return `Access denied`
	}

	if utils.StringInSlice(columns, `*`) {
		for _, col := range rows {
			queryColumns = append(queryColumns, col[columnNameKey])
			columnNames = append(columnNames, col[columnNameKey])
		}
	} else {
		if !utils.StringInSlice(columns, `id`) {
			columns = append(columns, `id`)
		}
		columnNames = make([]string, len(columns))
		copy(columnNames, columns)
		queryColumns = strings.Split(smart.PrepareColumns(columns), ",")
	}

	for i, col := range queryColumns {
		col = strings.Trim(col, `"`)
		switch columnTypes[col] {
		case "bytea":
			extendedColumns[col] = columnTypeBlob
			queryColumns[i] = dbfindExpressionBlob(col)
			break
		case "text", "varchar", "character varying":
			if cutoffColumns[col] {
				extendedColumns[col] = columnTypeLongText
				queryColumns[i] = dbfindExpressionLongText(col)
			}
			break
		}
	}
	for i, field := range queryColumns {
		if !strings.ContainsAny(field, `:.>"`) {
			queryColumns[i] = `"` + field + `"`
		}
	}
	for i, key := range columnNames {
		if strings.Contains(key, `->`) {
			columnNames[i] = strings.Replace(key, `->`, `.`, -1)
		}
		columnNames[i] = strings.TrimSpace(columnNames[i])
	}
	if par.Node.Attr[`countvar`] != nil {
		var count int64
		err = sqldb.GetDB(nil).Table(tblname).Where(where).Count(&count).Error
		if err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Debug("selecting count from table in DBFind")
		}
		countStr := converter.Int64ToStr(count)
		par.Node.Attr[`count`] = countStr
		setVar(par.Workspace, par.Node.Attr[`countvar`].(string), countStr)
		delete(par.Node.Attr, `countvar`)
	}
	if len(where) > 0 {
		where = ` where ` + where
	}
	list, err := sc.DbTransaction.GetAllTransaction(`select `+strings.Join(queryColumns, `, `)+` from "`+tblname+`"`+
		where+order+offset, limit)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting all from db")
		return err.Error()
	}
	data := make([][]string, 0)
	types := make([]string, 0)
	lencol := 0
	defcol := 0
	for _, item := range list {
		if lencol == 0 {
			for _, key := range columnNames {
				if v, ok := extendedColumns[key]; ok {
					types = append(types, v)
				} else {
					types = append(types, columnTypeText)
				}
			}
			defcol = len(columnNames)
			if par.Node.Attr[`customs`] != nil {
				for _, v := range par.Node.Attr[`customs`].([]string) {
					columnNames = append(columnNames, v)
					types = append(types, `tags`)
				}
			}
			lencol = len(columnNames)
		}
		row := make([]string, lencol)
		for i, icol := range columnNames {
			var ival string
			if i < defcol {
				ival = item[icol]
				if ival == `NULL` {
					ival = ``
				}

				switch extendedColumns[icol] {
				case columnTypeBlob:
					link := &valueLink{id: item["id"], column: icol, table: tblname, hash: ival, title: ival}
					ival, err = link.marshal()
					if err != nil {
						return err.Error()
					}
					item[icol] = link.link()
					break
				case columnTypeLongText:
					var res []string
					err = json.Unmarshal([]byte(ival), &res)
					if err != nil {
						log.WithFields(log.Fields{"type": consts.JSONUnmarshallError, "error": err}).Error("unmarshalling long text params from JSON")
						return err.Error()
					}
					link := &valueLink{id: item["id"], column: icol, table: tblname, hash: res[1], title: res[0]}
					ival, err = link.marshal()
					if err != nil {
						return err.Error()
					}
					break
				}
			} else {
				root := node{}
				for key, val := range item {
					(*par.Workspace.Vars)[key] = Var{Value: val}
				}
				process(par.Node.Attr[`custombody`].([]string)[i-defcol], &root, par.Workspace)
				for key := range item {
					delete(*par.Workspace.Vars, key)
				}
				out, err := json.Marshal(root.Children)
				if err == nil {
					ival = macro(string(out), mapToVar(item))
				} else {
					log.WithFields(log.Fields{"type": consts.JSONMarshallError, "error": err}).Error("marshalling root children to JSON")
				}
			}
			if par.Node.Attr[`prefix`] != nil {
				setVar(par.Workspace, prefix+`_`+strings.Replace(icol, `.`, `_`, -1), ival)
			}
			row[i] = ival
		}
		data = append(data, row)
	}
	if perm != nil && len(perm[`filter`]) > 0 {
		result := make([]any, len(data))
		for i, item := range data {
			row := make(map[string]string)
			for j, col := range columnNames {
				row[col] = item[j]
			}
			result[i] = reflect.ValueOf(row).Interface()
		}
		fltResult, err := sc.VM.EvalIf(perm[`filter`], uint32(sc.TxSmart.EcosystemID),
			map[string]any{
				`data`:         result,
				`ecosystem_id`: sc.TxSmart.EcosystemID,
				`key_id`:       sc.TxSmart.KeyID, `sc`: sc,
				`block_time`: 0, `time`: sc.Timestamp})
		if err != nil || !fltResult {
			return `Access denied`
		}
		for i := range data {
			for j, col := range columnNames {
				data[i][j] = result[i].(map[string]string)[col]
			}
		}
	}
	setAllAttr(par)
	delete(par.Node.Attr, `customs`)
	delete(par.Node.Attr, `custombody`)
	delete(par.Node.Attr, `prefix`)
	par.Node.Attr[`columns`] = &columnNames
	par.Node.Attr[`types`] = &types
	par.Node.Attr[`data`] = &data
	newSource(par)
	par.Owner.Children = append(par.Owner.Children, par.Node)
	return ``
}

func compositeTag(par parFunc) string {
	setAllAttr(par)
	if len((*par.Pars)[`Name`]) == 0 {
		return ``
	}
	if par.Owner.Attr[`composites`] == nil {
		par.Owner.Attr[`composites`] = make([]string, 0)
		par.Owner.Attr[`compositedata`] = make([]string, 0)
	}
	par.Owner.Attr[`composites`] = append(par.Owner.Attr[`composites`].([]string),
		macro((*par.Pars)[`Name`], par.Workspace.Vars))
	par.Owner.Attr[`compositedata`] = append(par.Owner.Attr[`compositedata`].([]string),
		macro((*par.Pars)[`Data`], par.Workspace.Vars))
	return ``
}

func errredirTag(par parFunc) string {
	setAllAttr(par)
	if len((*par.Pars)[`ErrorID`]) == 0 {
		return ``
	}
	if par.Owner.Attr[`errredirect`] == nil {
		par.Owner.Attr[`errredirect`] = make(map[string]map[string]any)
	}
	par.Owner.Attr[`errredirect`].(map[string]map[string]any)[(*par.Pars)[`ErrorID`]] =
		par.Node.Attr
	return ``
}

func popupTag(par parFunc) string {
	setAllAttr(par)

	width := converter.StrToInt((*par.Pars)[`Width`])
	if width < 1 || width > 100 {
		return ``
	}

	par.Owner.Attr[`popup`] = par.Node.Attr
	return ``
}

func customTag(par parFunc) string {
	setAllAttr(par)
	if len((*par.Pars)[`Column`]) == 0 || len((*par.Pars)[`Body`]) == 0 {
		return ``
	}
	if par.Owner.Attr[`customs`] == nil {
		par.Owner.Attr[`customs`] = make([]string, 0)
		par.Owner.Attr[`custombody`] = make([]string, 0)
	}
	par.Owner.Attr[`customs`] = append(par.Owner.Attr[`customs`].([]string), par.Node.Attr[`column`].(string))
	par.Owner.Attr[`custombody`] = append(par.Owner.Attr[`custombody`].([]string), (*par.Pars)[`Body`])
	return ``
}

func customTagFull(par parFunc) string {
	setAllAttr(par)
	process((*par.Pars)[`Body`], par.Node, par.Workspace)
	par.Owner.Tail = append(par.Owner.Tail, par.Node)
	return ``
}

func tailTag(par parFunc) string {
	setAllAttr(par)
	for key, v := range par.Node.Attr {
		switch v.(type) {
		case string:
			par.Owner.Attr[key] = macro(v.(string), par.Workspace.Vars)
		default:
			par.Owner.Attr[key] = v
		}
	}
	return ``
}

func showHideTag(par parFunc, action string) string {
	setAllAttr(par)
	cond := par.Node.Attr[`condition`]
	if v, ok := cond.(string); ok {
		val := make(map[string]string)
		items := strings.Split(v, `,`)
		for _, item := range items {
			lr := strings.SplitN(strings.TrimSpace(item), `=`, 2)
			key := strings.TrimSpace(lr[0])
			if len(lr) == 2 {
				val[key] = macro(strings.TrimSpace(lr[1]), par.Workspace.Vars)
			} else {
				val[key] = ``
			}
		}
		if _, ok := par.Owner.Attr[action]; ok {
			par.Owner.Attr[action] = append(par.Owner.Attr[action].([]map[string]string), val)
		} else {
			par.Owner.Attr[action] = []map[string]string{val}
		}
	}
	return ``
}

func showTag(par parFunc) string {
	return showHideTag(par, `show`)
}

func hideTag(par parFunc) string {
	return showHideTag(par, `hide`)
}

func includeTag(par parFunc) string {
	if len((*par.Pars)[`Name`]) >= 0 && len(getVar(par.Workspace, `_include`)) < 5 {
		bi := &sqldb.Snippet{}
		name := macro((*par.Pars)[`Name`], par.Workspace.Vars)
		ecosystem, tblname := converter.ParseName(name)
		prefix := getVar(par.Workspace, `ecosystem_id`)
		if ecosystem != 0 {
			prefix = converter.Int64ToStr(ecosystem)
			name = tblname
		}
		bi.SetTablePrefix(prefix)
		found, err := bi.Get(name)
		if err != nil {
			log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting snippet by name")
			return err.Error()
		}
		if !found {
			log.WithFields(log.Fields{"type": consts.NotFound, "name": (*par.Pars)[`Name`]}).Error("include snippet not found")
			return fmt.Sprintf("Inlcude %s has not been found", (*par.Pars)[`Name`])
		}
		if len(bi.Value) > 0 {
			root := node{}
			setVar(par.Workspace, `_include`, getVar(par.Workspace, `_include`)+`1`)
			process(bi.Value, &root, par.Workspace)
			include := getVar(par.Workspace, `_include`)
			setVar(par.Workspace, `_include`, include[:len(include)-1])
			for _, item := range root.Children {
				par.Owner.Children = append(par.Owner.Children, item)
			}
		}
	}
	return ``
}

func setvarTag(par parFunc) string {
	if len((*par.Pars)[`Name`]) > 0 {
		if strings.ContainsAny((*par.Pars)[`Value`], `({`) {
			(*par.Pars)[`Value`] = processToText(par, (*par.Pars)[`Value`])
		}
		setVar(par.Workspace, (*par.Pars)[`Name`], macroReplace((*par.Pars)[`Value`], par.Workspace.Vars))
	}
	return ``
}

func varasisTag(par parFunc) string {
	key := (*par.Pars)[`Name`]
	if len(key) > 0 {
		value := (*par.Pars)[`Value`]
		if strings.HasPrefix(value, `#`) {
			if v, ok := (*par.Workspace.Vars)[strings.Trim(value, `#`)]; ok {
				value = v.Value
			}
		} else if v, ok := (*par.Workspace.Vars)[value]; ok {
			value = v.Value
		}
		(*par.Workspace.Vars)[key] = Var{Value: value, AsIs: true}
	}
	return ``
}

func getvarTag(par parFunc) string {
	if len((*par.Pars)[`Name`]) > 0 {
		return macro(getVar(par.Workspace, (*par.Pars)[`Name`]), par.Workspace.Vars)
	}
	return ``
}

func tableTag(par parFunc) string {
	defaultTag(par)
	defaultTail(par, `table`)
	if len((*par.Pars)[`Columns`]) > 0 {
		imap := make([]map[string]string, 0)
		for _, v := range strings.Split((*par.Pars)[`Columns`], `,`) {
			v = macro(strings.TrimSpace(v), par.Workspace.Vars)
			if off := strings.IndexByte(v, '='); off == -1 {
				imap = append(imap, map[string]string{`Title`: v, `Name`: v})
			} else {
				imap = append(imap, map[string]string{`Title`: strings.TrimSpace(v[:off]), `Name`: strings.TrimSpace(v[off+1:])})
			}
		}
		if len(imap) > 0 {
			par.Node.Attr[`columns`] = imap
		}
	}
	return ``
}

func validateTag(par parFunc) string {
	setAllAttr(par)
	par.Owner.Attr[`validate`] = par.Node.Attr
	return ``
}

func validateFull(par parFunc) string {
	setAllAttr(par)
	par.Owner.Tail = append(par.Owner.Tail, par.Node)
	return ``
}

func defaultTail(par parFunc, tag string) {
	if par.Tails != nil {
		for _, v := range *par.Tails {
			name := (*v)[len(*v)-1]
			curFunc := tails[tag].Tails[string(name)].tplFunc
			pars := (*v)[:len(*v)-1]
			callFunc(&curFunc, par.Node, par.Workspace, &pars, nil)
		}
	}
}

func defaultTailTag(par parFunc) string {
	defaultTag(par)
	defaultTail(par, par.Node.Tag)
	return ``
}

func buttonTag(par parFunc) string {
	defaultTag(par)
	defaultTail(par, `button`)
	defer func() {
		delete(par.Node.Attr, `composites`)
		delete(par.Node.Attr, `compositedata`)
	}()
	if par.Node.Attr[`composites`] != nil {
		composites := make([]Composite, 0)
		for i, name := range par.Node.Attr[`composites`].([]string) {
			var data any
			input := par.Node.Attr[`compositedata`].([]string)[i]
			if len(input) > 0 {
				if err := json.Unmarshal([]byte(input), &data); err != nil {
					log.WithFields(log.Fields{"type": consts.JSONUnmarshallError, "source": input}).Error("on button tag unmarshaling content")
					return err.Error()
				}
			}
			composites = append(composites, Composite{Name: name, Data: data})
		}
		par.Node.Attr[`composite`] = &composites
	}
	return ``
}

func ifTag(par parFunc) string {
	cond := ifValue((*par.Pars)[`Condition`], par.Workspace)
	if cond {
		process((*par.Pars)[`Body`], par.Node, par.Workspace)
		for _, item := range par.Node.Children {
			par.Owner.Children = append(par.Owner.Children, item)
		}
	}
	if !cond && par.Tails != nil {
		for _, v := range *par.Tails {
			name := (*v)[len(*v)-1]
			curFunc := tails[`if`].Tails[string(name)].tplFunc
			pars := (*v)[:len(*v)-1]
			callFunc(&curFunc, par.Owner, par.Workspace, &pars, nil)
			if getVar(par.Workspace, `_cond`) == `1` {
				setVar(par.Workspace, `_cond`, `0`)
				break
			}
		}
	}
	return ``
}

func ifFull(par parFunc) string {
	setAttr(par, `Condition`)
	par.Owner.Children = append(par.Owner.Children, par.Node)
	if par.Tails != nil {
		for _, v := range *par.Tails {
			name := (*v)[len(*v)-1]
			curFunc := tails[`if`].Tails[string(name)].tplFunc
			pars := (*v)[:len(*v)-1]
			callFunc(&curFunc, par.Node, par.Workspace, &pars, nil)
		}
	}
	return ``
}

func elseifTag(par parFunc) string {
	cond := ifValue((*par.Pars)[`Condition`], par.Workspace)
	if cond {
		process((*par.Pars)[`Body`], par.Node, par.Workspace)
		for _, item := range par.Node.Children {
			par.Owner.Children = append(par.Owner.Children, item)
		}
		setVar(par.Workspace, `_cond`, `1`)
	}
	return ``
}

func elseifFull(par parFunc) string {
	setAttr(par, `Condition`)
	par.Owner.Tail = append(par.Owner.Tail, par.Node)
	return ``
}

func elseTag(par parFunc) string {
	for _, item := range par.Node.Children {
		par.Owner.Children = append(par.Owner.Children, item)
	}
	return ``
}

func elseFull(par parFunc) string {
	par.Owner.Tail = append(par.Owner.Tail, par.Node)
	return ``
}

func dateTimeTag(par parFunc) string {
	datetime := par.ParamWithMacros("DateTime")
	if len(datetime) == 0 || datetime[0] < '0' || datetime[0] > '9' {
		return ``
	}
	value := datetime
	defTime := `1970-01-01T00:00:00`
	lenTime := len(datetime)
	if lenTime < len(defTime) {
		datetime += defTime[lenTime:]
	}
	itime, err := time.Parse(`2006-01-02T15:04:05`, strings.Replace(datetime[:19], ` `, `T`, -1))
	if err != nil {
		unix := converter.StrToInt64(value)
		if unix > 0 {
			itime = time.Unix(unix, 0)
		} else {
			return err.Error()
		}
	}
	format := par.ParamWithMacros("Format")
	if len(format) == 0 {
		format, _ = language.LangText(nil, `timeformat`,
			converter.StrToInt(getVar(par.Workspace, `ecosystem_id`)), getVar(par.Workspace, `lang`))
		if format == `timeformat` {
			format = `2006-01-02 15:04:05`
		}
	} else {
		format = macro(format, par.Workspace.Vars)
	}
	format = strings.Replace(format, `YYYY`, `2006`, -1)
	format = strings.Replace(format, `YY`, `06`, -1)
	format = strings.Replace(format, `MM`, `01`, -1)
	format = strings.Replace(format, `DD`, `02`, -1)
	format = strings.Replace(format, `HH`, `15`, -1)
	format = strings.Replace(format, `MI`, `04`, -1)
	format = strings.Replace(format, `SS`, `05`, -1)

	locationName := par.ParamWithMacros("Location")
	if len(locationName) > 0 {
		loc, err := time.LoadLocation(locationName)
		if err != nil {
			return err.Error()
		}
		itime = itime.In(loc)
	}

	return itime.Format(format)
}

func cmpTimeTag(par parFunc) string {
	prepare := func(val string) string {
		val = strings.Replace(macro(val, par.Workspace.Vars), `T`, ` `, -1)
		if len(val) > 19 {
			val = val[:19]
		}
		return val
	}
	left := prepare((*par.Pars)[`Time1`])
	right := prepare((*par.Pars)[`Time2`])
	if left == right {
		return `0`
	}
	if left < right {
		return `-1`
	}
	return `1`
}

type byFirst [][]string

func (s byFirst) Len() int {
	return len(s)
}
func (s byFirst) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s byFirst) Less(i, j int) bool {
	return strings.Compare(s[i][0], s[j][0]) < 0
}

func jsontosourceTag(par parFunc) string {
	setAllAttr(par)
	var prefix string
	if par.Node.Attr[`prefix`] != nil {
		prefix = par.Node.Attr[`prefix`].(string) + `_`
	}
	data := make([][]string, 0, 16)
	cols := []string{prefix + `key`, prefix + `value`}
	types := []string{`text`, `text`}
	var out map[string]any
	dataVal := macro((*par.Pars)[`Data`], par.Workspace.Vars)
	if len(dataVal) > 0 {
		json.Unmarshal([]byte(macro((*par.Pars)[`Data`], par.Workspace.Vars)), &out)
	}
	for key, item := range out {
		if item == nil {
			item = ``
		}
		var value string
		switch v := item.(type) {
		case map[string]any:
			var keys, values []string
			for k := range v {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				values = append(values, fmt.Sprintf(`%q:%q`, k, v[k]))
			}
			value = `{` + strings.Join(values, ",\r\n") + `}`
		default:
			value = fmt.Sprint(item)
		}
		data = append(data, []string{key, value})
	}
	sort.Sort(byFirst(data))
	setAllAttr(par)
	par.Node.Attr[`columns`] = &cols
	par.Node.Attr[`types`] = &types
	par.Node.Attr[`data`] = &data
	newSource(par)
	par.Owner.Children = append(par.Owner.Children, par.Node)
	return ``
}

func arraytosourceTag(par parFunc) string {
	setAllAttr(par)

	var prefix string
	if par.Node.Attr[`prefix`] != nil {
		prefix = par.Node.Attr[`prefix`].(string) + `_`
	}

	data := make([][]string, 0, 16)
	cols := []string{prefix + `key`, prefix + `value`}
	types := []string{`text`, `text`}
	for key, item := range splitArray([]rune(macro((*par.Pars)[`Data`], par.Workspace.Vars))) {
		data = append(data, []string{fmt.Sprint(key), item})
	}
	setAllAttr(par)
	par.Node.Attr[`columns`] = &cols
	par.Node.Attr[`types`] = &types
	par.Node.Attr[`data`] = &data
	newSource(par)
	par.Owner.Children = append(par.Owner.Children, par.Node)
	return ``
}

func chartTag(par parFunc) string {
	defaultTag(par)
	defaultTail(par, "chart")

	if len((*par.Pars)["Colors"]) > 0 {
		colors := strings.Split(macro((*par.Pars)["Colors"], par.Workspace.Vars), ",")
		for i, v := range colors {
			colors[i] = strings.TrimSpace(v)
		}
		par.Node.Attr["colors"] = colors
	}

	return ""
}

func rangeTag(par parFunc) string {
	setAllAttr(par)
	step := int64(1)
	data := make([][]string, 0, 32)
	from := converter.StrToInt64(macro((*par.Pars)["From"], par.Workspace.Vars))
	to := converter.StrToInt64(macro((*par.Pars)["To"], par.Workspace.Vars))
	if len((*par.Pars)["Step"]) > 0 {
		step = converter.StrToInt64(macro((*par.Pars)["Step"], par.Workspace.Vars))
	}
	if step > 0 && from < to {
		for i := from; i < to; i += step {
			data = append(data, []string{converter.Int64ToStr(i)})
		}
	} else if step < 0 && from > to {
		for i := from; i > to; i += step {
			data = append(data, []string{converter.Int64ToStr(i)})
		}
	}
	delete(par.Node.Attr, `from`)
	delete(par.Node.Attr, `to`)
	delete(par.Node.Attr, `step`)
	par.Node.Attr[`columns`] = &[]string{"id"}
	par.Node.Attr[`data`] = &data
	newSource(par)
	par.Owner.Children = append(par.Owner.Children, par.Node)
	return ``
}

func imageTag(par parFunc) string {
	(*par.Pars)["Src"] = parseArg((*par.Pars)["Src"], par.Workspace)
	defaultTag(par)
	defaultTail(par, par.Node.Tag)
	return ``
}

func binaryTag(par parFunc) string {
	var ecosystemID string

	defaultTail(par, `binary`)
	if par.Node.Attr[`ecosystem`] != nil {
		ecosystemID = par.Node.Attr[`ecosystem`].(string)
	} else {
		ecosystemID = getVar(par.Workspace, `ecosystem_id`)
	}
	binary := &sqldb.Binary{}
	binary.SetTablePrefix(ecosystemID)

	var (
		ok  bool
		err error
	)

	if par.Node.Attr["id"] != nil {
		ok, err = binary.GetByID(converter.StrToInt64(macro(par.Node.Attr["id"].(string), par.Workspace.Vars)))
	} else {
		ok, err = binary.Get(
			converter.StrToInt64(par.ParamWithMacros("AppID")),
			par.ParamWithMacros("Account"),
			par.ParamWithMacros("Name"),
		)
	}

	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting record from db")
		return err.Error()
	}

	if ok {
		return binary.Link()
	}

	return ""
}

func columntypeTag(par parFunc) string {
	if len((*par.Pars)["Table"]) > 0 && len((*par.Pars)["Column"]) > 0 {
		tableName := macro((*par.Pars)[`Table`], par.Workspace.Vars)
		columnName := macro((*par.Pars)[`Column`], par.Workspace.Vars)
		tblname := qb.GetTableName(par.Workspace.SmartContract.TxSmart.EcosystemID, tableName)
		colType, err := par.Workspace.SmartContract.DbTransaction.GetColumnType(tblname, columnName)
		if err == nil {
			return colType
		}
		return err.Error()
	}
	return ``
}

func getHistoryTag(par parFunc) string {
	setAllAttr(par)
	var rollID int64
	if len((*par.Pars)["RollbackId"]) > 0 {
		rollID = converter.StrToInt64(macro((*par.Pars)[`RollbackId`], par.Workspace.Vars))
	}
	if len((*par.Pars)["Name"]) == 0 {
		return ``
	}
	table := macro((*par.Pars)["Name"], par.Workspace.Vars)
	list, err := smart.GetHistoryRaw(nil, converter.StrToInt64(getVar(par.Workspace, `ecosystem_id`)),
		table, converter.StrToInt64(macro((*par.Pars)[`Id`], par.Workspace.Vars)), rollID)
	if err != nil {
		return err.Error()
	}

	colsList, err := par.Workspace.SmartContract.DbTransaction.GetAllColumnTypes(getVar(par.Workspace, `ecosystem_id`) + "_" + table)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("getting column types from db")
		return err.Error()
	}

	cols := make([]string, 0, len(colsList))
	typesCol := make([]string, 0, len(colsList))
	for _, v := range colsList {

		cols = append(cols, v[columnNameKey])
		typesCol = append(typesCol, `text`)
	}

	data := make([][]string, 0)
	if len(list) > 0 {
		for i := range list {
			item := list[i].(*types.Map)
			items := make([]string, len(cols))
			for ind, key := range cols {
				var val string
				if v, found := item.Get(key); found {
					val = v.(string)
				}
				if val == `NULL` {
					val = ``
				}
				items[ind] = val
			}
			data = append(data, items)
		}
	}
	par.Node.Attr[`columns`] = &cols
	par.Node.Attr[`types`] = &typesCol
	par.Node.Attr[`data`] = &data
	newSource(par)
	par.Owner.Children = append(par.Owner.Children, par.Node)
	return ``
}
