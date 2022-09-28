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
	Objects map[string]*ObjInfo
	Type    ObjectType
	Owner   *OwnerInfo
	// Types that are valid to be assigned to Info:
	//	*FuncInfo
	//	*ContractInfo
	Info     isCodeBlockInfo
	Parent   *CodeBlock
	Vars     []reflect.Type
	Code     ByteCodes
	Children CodeBlocks
}

type isCodeBlockInfo interface {
	isCodeBlockInfo()
}

func (*FuncInfo) isCodeBlockInfo()     {}
func (*ContractInfo) isCodeBlockInfo() {}

func (m *CodeBlock) GetInfo() isCodeBlockInfo {
	if m != nil {
		return m.Info
	}
	return nil
}

func (m *CodeBlock) GetFuncInfo() *FuncInfo {
	if x, ok := m.GetInfo().(*FuncInfo); ok {
		return x
	}
	return nil
}

func (m *CodeBlock) GetContractInfo() *ContractInfo {
	if x, ok := m.GetInfo().(*ContractInfo); ok {
		return x
	}
	return nil
}

// ByteCode stores a command and an additional parameter.
type ByteCode struct {
	Cmd    uint16
	Line   uint16
	Lexeme Lexeme
	Value  any
}

// CodeBlocks is a slice of blocks
type CodeBlocks []*CodeBlock

func (bs *CodeBlocks) push(x any) {
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

func (bs *ByteCodes) push(x any) {
	*bs = append(*bs, x.(*ByteCode))
}

func (bs *ByteCodes) peek() *ByteCode {
	bsLen := len(*bs)
	if bsLen == 0 {
		return nil
	}
	return (*bs)[bsLen-1]
}

func newByteCode(cmd uint16, line uint16, value any) *ByteCode {
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
	Type ObjectType
	// Types that are valid to be assigned to Value:
	//	*CodeBlock
	//	*ExtFuncInfo
	//	*ObjInfo_Variable
	//	*ObjInfo_ExtendVariable
	Value isObjInfoValue
}

type isObjInfoValue interface {
	isObjInfoValue()
}
type ObjInfo_Variable struct {
	Name  string
	Index int
}
type ObjInfo_ExtendVariable struct {
	//object extend variable name
	Name string
}

func (*CodeBlock) isObjInfoValue()              {}
func (*ExtFuncInfo) isObjInfoValue()            {}
func (*ObjInfo_Variable) isObjInfoValue()       {}
func (*ObjInfo_ExtendVariable) isObjInfoValue() {}

func (m *ObjInfo) GetValue() isObjInfoValue {
	if m != nil {
		return m.Value
	}
	return nil
}

func (m *ObjInfo) GetCodeBlock() *CodeBlock {
	if x, ok := m.GetValue().(*CodeBlock); ok {
		return x
	}
	return nil
}

func (m *ObjInfo) GetExtFuncInfo() *ExtFuncInfo {
	if x, ok := m.GetValue().(*ExtFuncInfo); ok {
		return x
	}
	return nil
}

func (m *ObjInfo) GetVariable() *ObjInfo_Variable {
	if x, ok := m.GetValue().(*ObjInfo_Variable); ok {
		return x
	}
	return nil
}

func (m *ObjInfo) GetExtendVariable() *ObjInfo_ExtendVariable {
	if x, ok := m.GetValue().(*ObjInfo_ExtendVariable); ok {
		return x
	}
	return nil
}

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
			b.Objects[key] = &ObjInfo{Type: ObjectType_ExtFunc, Value: data}
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
		block = ret.GetCodeBlock()
	}
	return
}

func (block *CodeBlock) parentContractCost() int64 {
	var cost int64
	parent := block.Parent.GetContractInfo()
	if parent != nil {
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
			blockItem.GetFuncInfo().CanWrite = true
		}
		if blockItem.Type == ObjectType_Contract {
			blockItem.GetContractInfo().CanWrite = true
		}
	}
}
func (ret *ObjInfo) getInParams() int {
	if ret.Type == ObjectType_ExtFunc {
		return len(ret.GetExtFuncInfo().Params)
	}
	return len(ret.GetCodeBlock().GetFuncInfo().Params)
}
