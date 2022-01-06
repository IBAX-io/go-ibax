package script

import (
	"reflect"
	"strings"
)

/* Byte code could be described as a tree where functions and contracts are on the top level and
nesting goes further according to nesting of bracketed. Tree nodes are structures of
'CodeBlock' type. For instance,
 func a {
	 if b {
		 while d {

		 }
	 }
	 if c {
	 }
 }
 will be compiled into CodeBlock(a) which will have two child blocks CodeBlock (b) and CodeBlock (c) that
 are responsible for executing bytecode inside if. CodeBlock (b) will have a child CodeBlock (d) with
 a cycle.
*/

// CodeBlock contains all information about compiled block {...} and its children
type CodeBlock struct {
	Objects  map[string]*ObjInfo
	Type     ObjectType
	Owner    *OwnerInfo
	Info     *codeBlockInfo
	Parent   *CodeBlock
	Vars     []reflect.Type
	Code     ByteCodes
	Children CodeBlocks
}

type codeBlockInfo struct{ i interface{} }

func newCodeBlockInfo(i interface{}) *codeBlockInfo  { return &codeBlockInfo{i: i} }
func (i *codeBlockInfo) FuncInfo() *FuncInfo         { return i.i.(*FuncInfo) }
func (i *codeBlockInfo) Uint32() uint32              { return i.i.(uint32) }
func (i *codeBlockInfo) ContractInfo() *ContractInfo { return i.i.(*ContractInfo) }
func (i *codeBlockInfo) IsContractInfo() (*ContractInfo, bool) {
	v, ok := i.i.(*ContractInfo)
	return v, ok
}

// ByteCode stores a command and an additional parameter.
type ByteCode struct {
	Cmd   uint16
	Line  uint16
	Value interface{}
}

// CodeBlocks is a slice of blocks
type CodeBlocks []*CodeBlock

func (bs *CodeBlocks) push(x interface{}) {
	*bs = append(*bs, x.(*CodeBlock))
}

func (bs *CodeBlocks) peek() *CodeBlock {
	bsLen := len(*bs)
	if bsLen == 0 {
		return nil
	}
	return (*bs)[bsLen-1]
}

func (bs *CodeBlocks) get(idx int) *CodeBlock {
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
	Value *objInfoValue
}

type objInfoValue struct{ v interface{} }

func newObjInfoValue(v interface{}) *objInfoValue { return &objInfoValue{v: v} }
func (i *objInfoValue) CodeBlock() *CodeBlock     { return i.v.(*CodeBlock) }
func (i *objInfoValue) ExtFuncInfo() *ExtFuncInfo { return i.v.(*ExtFuncInfo) }
func (i *objInfoValue) Int() int                  { return i.v.(int) }
func (i *objInfoValue) String() string            { return i.v.(string) }

func NewCodeBlock() *CodeBlock {
	b := &CodeBlock{
		Objects: make(map[string]*ObjInfo),
		// Reserved 256 indexes for system purposes
		Children: make(CodeBlocks, 256, 1024),
	}
	b.Extend(NewExtendData())
	return b
}

// Extend sets the extended variables and functions
func (b *CodeBlock) Extend(ext *ExtendData) {
	for key, item := range ext.Objects {
		fobj := reflect.ValueOf(item).Type()
		switch fobj.Kind() {
		case reflect.Func:
			_, canWrite := ext.WriteFuncs[key]
			data := &ExtFuncInfo{
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
			b.Objects[key] = &ObjInfo{Type: ObjectType_ExtFunc, Value: newObjInfoValue(data)}
		}
	}
}

func (b *CodeBlock) getObjByNameExt(name string, state uint32) (ret *ObjInfo) {
	sname := StateName(state, name)
	if ret = b.getObjByName(name); ret == nil && len(sname) > 0 {
		ret = b.getObjByName(sname)
	}
	return
}

func (block *CodeBlock) getObjByName(name string) (ret *ObjInfo) {
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
		block = ret.Value.CodeBlock()
	}
	return
}

func (block *CodeBlock) parentContractCost() int64 {
	var cost int64
	if parent, ok := block.Parent.Info.IsContractInfo(); ok {
		cost += int64(len(block.Parent.Objects) * CostCall)
		cost += int64(len(parent.Settings) * CostCall)
		if parent.Tx != nil {
			cost += int64(len(*parent.Tx) * CostExtend)
		}
	}
	return cost
}

func (block *CodeBlock) isParentContract() bool {
	if block.Parent != nil && block.Parent.Type == ObjectType_Contract {
		return true
	}
	return false
}

func setWritable(block *CodeBlocks) {
	for i := len(*block) - 1; i >= 0; i-- {
		blockItem := (*block)[i]
		if blockItem.Type == ObjectType_Func {
			blockItem.Info.FuncInfo().CanWrite = true
		}
		if blockItem.Type == ObjectType_Contract {
			blockItem.Info.ContractInfo().CanWrite = true
		}
	}
}
func (ret *ObjInfo) getInParams() int {
	if ret.Type == ObjectType_ExtFunc {
		return len(ret.Value.ExtFuncInfo().Params)
	}
	return len(ret.Value.CodeBlock().Info.FuncInfo().Params)
}
