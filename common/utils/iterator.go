package utils

func IteratorSliceFn(args []interface{}, iterFn func(arg interface{})) {
	for _, arg := range args {
		iterFn(arg)
	}
}

func IteratorVariableFn(args ...interface{}) func(func(interface{})) {
	return func(iterFn func(arg interface{})) {
		for _, arg := range args {
			iterFn(arg)
		}
	}
}
