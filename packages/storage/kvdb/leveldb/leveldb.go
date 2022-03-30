package leveldb

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

var DBlevel *leveldb.DB
var GLeveldbIsactive bool

type levelDBGetterPutterDeleter interface {
	Get([]byte, *opt.ReadOptions) ([]byte, error)
	Put([]byte, []byte, *opt.WriteOptions) error
	Write(batch *leveldb.Batch, wo *opt.WriteOptions) error
	Delete([]byte, *opt.WriteOptions) error
	NewIterator(slice *util.Range, ro *opt.ReadOptions) iterator.Iterator
}

func GetLevelDB(tx *leveldb.Transaction) levelDBGetterPutterDeleter {
	if tx != nil {
		return tx
	}
	return DBlevel
}

func prefixFunc(prefix string) func([]byte) []byte {
	return func(hash []byte) []byte {
		return []byte(prefix + string(hash))
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

	return err
}

func Struct2Map(obj any) map[string]any {
	t := reflect.TypeOf(obj)
	v := reflect.ValueOf(obj)

	var data = make(map[string]any)
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
