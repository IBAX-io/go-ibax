package script

func (rt *RunTime) Stack() []any {
	return rt.stack
}

func (rt *RunTime) push(d any) {
	rt.stack = append(rt.stack, d)
}

func (rt *RunTime) pop() (ret any) {
	ret = rt.peek()
	rt.stack = rt.stack[:rt.len()-1]
	return
}

func (rt *RunTime) len() int {
	return len(rt.stack)
}

func (rt *RunTime) swap(n int) {
	rt.stack[rt.len()-n], rt.stack[rt.len()-1] = rt.peek(), rt.stack[rt.len()-n]
}

func (rt *RunTime) dup(n int) {
	rt.push(&rt.stack[rt.len()-n])
}

func (rt *RunTime) peek() any {
	return rt.stack[rt.len()-1]
}

func (rt *RunTime) getStack(idx int) any {
	if idx >= 0 && rt.len() > 0 && rt.len() > idx {
		return rt.stack[idx]
	}
	return nil
}

func (rt *RunTime) resetByIdx(idx int) {
	rt.stack = rt.stack[:idx]
}

func (rt *RunTime) popBlock() (ret *blockStack) {
	ret = rt.blocks[len(rt.blocks)-1]
	rt.blocks = rt.blocks[:len(rt.blocks)-1]
	return
}
