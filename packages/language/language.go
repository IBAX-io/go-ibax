/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

package language

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/IBAX-io/go-ibax/packages/consts"
	"github.com/IBAX-io/go-ibax/packages/converter"
	"github.com/IBAX-io/go-ibax/packages/storage/sqldb"

	log "github.com/sirupsen/logrus"
)

//cacheLang is cache for language, first level is lang_name, second is lang dictionary
type cacheLang struct {
	res map[string]*map[string]string
}

var (
	// LangList is the list of available languages. It stores two-bytes codes
	LangList []string
	lang     = make(map[int]*cacheLang)
	mutex    = &sync.RWMutex{}
)

// IsLang checks if there is a language with code name
func IsLang(code string) bool {
	if LangList == nil {
		return true
	}
	for _, val := range LangList {
		if val == code {
			return true
		}
	}
	return false
}

// DefLang returns the default language
func DefLang() string {
	if LangList == nil {
		return `en`
	}
	return LangList[0]
}

// UpdateLang updates language sources for the specified state
func UpdateLang(state int, name, value string) error {
	mutex.Lock()
	defer mutex.Unlock()
	if _, ok := lang[state]; !ok {
		lang[state] = &cacheLang{make(map[string]*map[string]string)}
	}
	var ires map[string]string
	err := json.Unmarshal([]byte(value), &ires)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.JSONUnmarshallError, "value": value, "error": err}).Error("Unmarshalling json")
		return err
	}
	for key, val := range ires {
		ires[strings.ToLower(key)] = val
	}
	if len(ires) > 0 {
		(*lang[state]).res[name] = &ires
	}
	return nil
}

// loadLang download the language sources from database for the state
func loadLang(transaction *sqldb.DbTransaction, state int) error {
	language := &sqldb.Language{}
	prefix := strconv.FormatInt(int64(state), 10)

	languages, err := language.GetAll(transaction, prefix)
	if err != nil {
		log.WithFields(log.Fields{"type": consts.DBError, "error": err}).Error("Error querying all languages")
		return err
	}
	list := make([]map[string]string, 0)
	for _, l := range languages {
		list = append(list, l.ToMap())
	}
	res := make(map[string]*map[string]string)
	for _, ilist := range list {
		var ires map[string]string
		err := json.Unmarshal([]byte(ilist[`res`]), &ires)
		if err != nil {
			log.WithFields(log.Fields{"type": consts.JSONUnmarshallError, "value": ilist["res"], "error": err}).Error("Unmarshalling json")
		}
		for key, val := range ires {
			ires[strings.ToLower(key)] = val
		}
		res[ilist[`name`]] = &ires
	}
	mutex.Lock()
	defer mutex.Unlock()
	if _, ok := lang[state]; !ok {
		lang[state] = &cacheLang{}
	}
	lang[state].res = res
	return nil
}

// LangText looks for the specified word through language sources and returns the meaning of the source
// if it is found. Search goes according to the languages specified in 'accept'
func LangText(transaction *sqldb.DbTransaction, in string, state int, accept string) (string, bool) {
	if strings.IndexByte(in, ' ') >= 0 || state == 0 {
		return in, false
	}
	ecosystem, name := converter.ParseName(in)
	if ecosystem != 0 {
		state = int(ecosystem)
		in = name
	}
	if state == 0 {
		return in, false
	}
	if _, ok := lang[state]; !ok {
		if err := loadLang(transaction, state); err != nil {
			return err.Error(), false
		}
	}
	mutex.RLock()
	defer mutex.RUnlock()
	langs := strings.Split(accept, `,`)
	if _, ok := (*lang[state]).res[in]; !ok {
		return in, false
	}
	if lres, ok := (*lang[state]).res[in]; ok {
		lng := DefLang()
		for _, val := range langs {
			val = strings.ToLower(val)
			if len(val) < 2 {
				break
			}
			if !IsLang(val[:2]) {
				continue
			}
			if len(val) >= 5 && val[2] == '-' {
				if _, ok := (*lres)[val[:5]]; ok {
					lng = val[:5]
					break
				}
			}
			if _, ok := (*lres)[val[:2]]; ok {
				lng = val[:2]
				break
			}
		}
		if len((*lres)[lng]) == 0 {
			for _, val := range *lres {
				return val, true
			}
		}
		return (*lres)[lng], true
	}
	return in, false
}

// LangMacro replaces all inclusions of $resname$ in the incoming text with the corresponding language resources,
// if they exist
func LangMacro(input string, state int, accept string) string {
	if !strings.ContainsRune(input, '$') {
		return input
	}
	syschar := '$'
	length := utf8.RuneCountInString(input)
	result := make([]rune, 0, length)
	isName := false
	name := make([]rune, 0, 128)
	clearname := func() {
		result = append(append(result, syschar), name...)
		isName = false
		name = name[:0]
	}
	for _, r := range input {
		if r != syschar {
			if isName {
				name = append(name, r)
				if len(name) > 64 || r < ' ' {
					clearname()
				}
			} else {
				result = append(result, r)
			}
			continue
		}
		if isName {
			value, ok := LangText(nil, string(name), state, accept)
			if ok {
				result = append(result, []rune(value)...)
				isName = false
			} else {
				result = append(append(result, syschar), name...)
			}
			name = name[:0]
		} else {
			isName = true
		}
	}
	if isName {
		result = append(append(result, syschar), name...)
	}

	return string(result)
}

// GetLang returns the first language from accept-language
func GetLang(state int, accept string) (lng string) {
	lng = DefLang()
	for _, val := range strings.Split(accept, `,`) {
		if len(val) < 2 {
			continue
		}
		if !IsLang(val[:2]) {
			continue
		}
		lng = val[:2]
		break
	}
	return
}
