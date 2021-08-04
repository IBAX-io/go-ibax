package model

import (
	"fmt"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
	"reflect"
	"strings"
)

var DBlevel *leveldb.DB
var GLeveldbIsactive bool

type levelDBGetterPutterDeleter interface {
	Get([]byte, *opt.ReadOptions) ([]byte, error)
	Put([]byte, []byte, *opt.WriteOptions) error
	Write(batch *leveldb.Batch, wo *opt.WriteOptions) error
	Delete([]byte, *opt.WriteOptions) error
	}
}

func prefixStringFunc(prefix string) func(key string) []byte {
	return func(key string) []byte {
		return []byte(prefix + key)
	}
}

func Init_leveldb(filename string) error {
	var err error
	DBlevel, err = leveldb.OpenFile(filename, nil)
	if err == nil {
		GLeveldbIsactive = true
	}

	//go Deal_MintCount()
	return err
}

func Struct2Map(obj interface{}) map[string]interface{} {
	t := reflect.TypeOf(obj)
	v := reflect.ValueOf(obj)

	var data = make(map[string]interface{})
	for i := 0; i < t.NumField(); i++ {
		data[t.Field(i).Name] = v.Field(i).Interface()
	}
	return data
}
func DBGetAllKey(prefix string, bvalue bool) (*[]string, error) {
	var (
		ret []string
		//key []string
	)
	found := prefix != "nil"
	iter := DBlevel.NewIterator(nil, nil)
	for iter.Next() {
		key := string(iter.Key())
		if found {
			if strings.HasPrefix(key, prefix) {
				if bvalue {
					value := string(iter.Value())
					s := fmt.Sprintf("Key[%s]=[%s]\n", key, value)
					ret = append(ret, s)
				} else {
					ret = append(ret, key)
				}
			}
		} else {
			if bvalue {
				value := string(iter.Value())
				s := fmt.Sprintf("Key[%s]=[%s]\n", key, value)
				ret = append(ret, s)
			} else {
				ret = append(ret, key)
			}
		}

	}
	iter.Release()
	return &ret, iter.Error()
}
