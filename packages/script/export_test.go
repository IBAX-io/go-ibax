package script

import "github.com/pkg/errors"

// CallExtendForTest calls the extended function name of vm as the byte code of a call does, with
// args, those the VM passes by itself left out, on the stack and extend the extended variables:
// it returns the stack after the call, the cost left and the error, that of a panic as RunCode
// tells it
func CallExtendForTest(vm *VM, name string, extend map[string]any, cost int64, args []any) (stack []any, left int64, err error) {
	obj := vm.Objects[name]
	rt := NewRunTime(vm, cost)
	rt.extend = extend
	for _, arg := range args {
		rt.push(arg)
	}
	cmd := uint16(cmdCall)
	if obj.GetExtFuncInfo().Variadic {
		rt.push(len(args))
		cmd = cmdCallVariadic
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = errors.Errorf(`runtime run code crashed: %v`, r)
			}
		}()
		err = rt.callFunc(cmd, obj)
	}()
	return append([]any(nil), rt.stack...), rt.cost, err
}

// ExtCallFallbacksForTest is the number of the calls an ExtCall refused for the types of their
// arguments
func ExtCallFallbacksForTest() int64 {
	return extCallFallbacks.Load()
}

// ExtendCallerForTest is a call of the extended function name of vm with args, which runs on one
// runtime each time it is called
func ExtendCallerForTest(vm *VM, name string, extend map[string]any, args []any) func() error {
	obj := vm.Objects[name]
	rt := NewRunTime(vm, 1<<62)
	rt.extend = extend
	cmd := uint16(cmdCall)
	variadic := obj.GetExtFuncInfo().Variadic
	if variadic {
		cmd = cmdCallVariadic
	}
	return func() error {
		for _, arg := range args {
			rt.push(arg)
		}
		if variadic {
			rt.push(len(args))
		}
		err := rt.callFunc(cmd, obj)
		rt.resetByIdx(0)
		return err
	}
}
