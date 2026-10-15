package script

import (
	"fmt"
	"sync"
	"testing"
)

// API requests evaluate conditions in goroutines of their own, each compiling the ones it meets
// first: none of them may fault the others, as an unguarded cache made the runtime stop the node
func TestEvalIfConcurrently(t *testing.T) {
	vm := NewVM()
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				// Half the conditions are shared by every goroutine, half are its own
				input := fmt.Sprintf("%d == %d", i, i)
				if i%2 == 1 {
					input = fmt.Sprintf("%d == %d", g*1000+i, g*1000+i)
				}
				ok, err := vm.EvalIf(input, 1, nil)
				if err != nil || !ok {
					errs <- fmt.Errorf("%s: %v, %v", input, ok, err)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
