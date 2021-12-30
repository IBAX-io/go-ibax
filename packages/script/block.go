package script

import (
	"reflect"
	"strings"
)

// Blocks is a slice of blocks
type Blocks []*Block

func (bs *Blocks) push(x interface{}) {
	*bs = append(*bs, x.(*Block))
}

func (bs *Blocks) peek() *Block {
	bsLen := len(*bs)
	if bsLen == 0 {
		return nil
	}
	return (*bs)[bsLen-1]
}

func (bs *Blocks) get(idx int) *Block {
	if idx >= 0 && len(*bs) > 0 && len(*bs) > idx {
		return (*bs)[idx]
	}
	return nil
}

// ByteCodes is the slice of ByteCode items
type ByteCodes []*ByteCode

func (bs *ByteCodes) push(x interface{}) {
	*bs = append(*bs, x.(*ByteCode))
}

func (bs *ByteCodes) peek() *ByteCode {
	bsLen := len(*bs)
	if bsLen == 0 {
		return nil
	}
	return (*bs)[bsLen-1]
}

// Block contains all information about compiled block {...} and its children
type Block struct {
	Objects  map[string]*ObjInfo
	Type     ObjectType
	Owner    *OwnerInfo
	Info     interface{}
	Parent   *Block
	Vars     []reflect.Type
	Code     ByteCodes
	Children Blocks
}

// ByteCode stores a command and an additional parameter.
type ByteCode struct {
	Cmd   uint16
	Line  uint16
	Value interface{}
}

func newByteCode(cmd uint16, line uint16, value interface{}) *ByteCode {
	return &ByteCode{Cmd: cmd, Line: line, Value: value}
}

// OwnerInfo storing info about owner
type OwnerInfo struct {
	StateID  uint32 `json:"state"`
	Active   bool   `json:"active"`
	TableID  int64  `json:"tableid"`
	WalletID int64  `json:"walletid"`
	TokenID  int64  `json:"tokenid"`
}

// ObjInfo is the common object type
type ObjInfo struct {
	Type  ObjectType
	Value interface{}
}

func NewBlock() *Block {
	b := &Block{
		Objects: make(map[string]*ObjInfo),
		// Reserved 256 indexes for system purposes
		Children: make(Blocks, 256, 1024),
	}
	b.Extend(NewExtendData())
	return b
}

// Extend sets the extended variables and functions
func (b *Block) Extend(ext *ExtendData) {
	for key, item := range ext.Objects {
		fobj := reflect.ValueOf(item).Type()
		switch fobj.Kind() {
		case reflect.Func:
			_, canWrite := ext.WriteFuncs[key]
			data := ExtFuncInfo{
				Name:     key,
				Params:   make([]reflect.Type, fobj.NumIn()),
				Results:  make([]reflect.Type, fobj.NumOut()),
				Auto:     make([]string, fobj.NumIn()),
				Variadic: fobj.IsVariadic(),
				Func:     item,
				CanWrite: canWrite}
			for i := 0; i < fobj.NumIn(); i++ {
				if isauto, ok := ext.AutoPars[fobj.In(i).String()]; ok {
					data.Auto[i] = isauto
				}
				data.Params[i] = fobj.In(i)
			}
			for i := 0; i < fobj.NumOut(); i++ {
				data.Results[i] = fobj.Out(i)
			}
			b.Objects[key] = &ObjInfo{Type: ObjectType_ExtFunc, Value: data}
		}
	}
}

func (b *Block) getObjByNameExt(name string, state uint32) (ret *ObjInfo) {
	sname := StateName(state, name)
	if ret = b.getObjByName(name); ret == nil && len(sname) > 0 {
		ret = b.getObjByName(sname)
	}
	return
}

func (block *Block) getObjByName(name string) (ret *ObjInfo) {
	var ok bool
	names := strings.Split(name, `.`)
	for i, name := range names {
		ret, ok = block.Objects[name]
		if !ok {
			return nil
		}
		if i == len(names)-1 {
			return
		}
		if ret.Type != ObjectType_Contract && ret.Type != ObjectType_Func {
			return nil
		}
		block = ret.Value.(*Block)
	}
	return
}
func (block *Block) parentContractCost() int64 {
	var cost int64
	if parent, ok := block.Parent.Info.(*ContractInfo); ok {
		cost += int64(len(block.Parent.Objects) * CostCall)
		cost += int64(len(parent.Settings) * CostCall)
		if parent.Tx != nil {
			cost += int64(len(*parent.Tx) * CostExtend)
		}
	}
	return cost
}

func (block *Block) isParentContract() bool {
	if block.Parent != nil && block.Parent.Type == ObjectType_Contract {
		return true
	}
	return false
}

func (ret *ObjInfo) getInParams() int {
	if ret.Type == ObjectType_ExtFunc {
		return len(ret.Value.(ExtFuncInfo).Params)
	}
	return len(ret.Value.(*Block).Info.(*FuncInfo).Params)
}
