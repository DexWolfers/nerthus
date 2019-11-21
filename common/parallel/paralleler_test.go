package parallel

import (
	"fmt"
	"github.com/pkg/errors"
	"testing"
	"time"
)

func TestParallel(t *testing.T) {
	pl := NewParalleler()
	pl.Async(func(args ...interface{}) {
		i := 2
		for {
			fmt.Println(">>>1.0>>>", i)
			time.Sleep(time.Millisecond * 800)
			i--
			if i <= 0 {
				break
			}
		}
		fmt.Println("1.0 finished")
	})
	pl.Async(func(args ...interface{}) {
		i := 3
		for {
			fmt.Println(">>>1.1>>>", i)
			time.Sleep(time.Millisecond * 700)
			i--
			if i <= 0 {
				break
			}
		}
		fmt.Println("1.1 finished")
	})
	pl.Async(func(args ...interface{}) {
		i := 4
		for {
			fmt.Println(">>>1.2>>>", i)
			time.Sleep(time.Millisecond * 600)
			i--
			if i <= 0 {
				break
			}
		}
		fmt.Println("1.2 finished")
	})
	pl.Wait()
	pl.Abort(errors.New("1111"))
	fmt.Println("all finished")
}
