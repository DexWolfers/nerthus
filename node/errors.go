// Author: @ysqi

package node

import (
	"errors"
	"fmt"
	"reflect"
	"syscall"
)

var (
	ErrDatadirUsed    = errors.New("datadir already used by another process")
	ErrNodeStopped    = errors.New("node not started")
	ErrNodeRunning    = errors.New("node already running")
	ErrServiceUnknown = errors.New("unknown service")

	datadirInUseErrnos = map[uint]bool{11: true, 32: true, 35: true}
)

func convertFileLockError(err error) error {
	if errno, ok := err.(syscall.Errno); ok && datadirInUseErrnos[uint(errno)] {
		return ErrDatadirUsed
	}
	return err
}

// DuplicateServiceError 重复启动同类型服务错误
type DuplicateServiceError struct {
	Kind reflect.Type
}

// Error  重复启动同类型服务错误信息
func (e *DuplicateServiceError) Error() string {
	return fmt.Sprintf("duplicate service: %v", e.Kind)
}

// StopError 节点停止操作失败错误
type StopError struct {
	Server   error
	Services map[reflect.Type]error
}

// Error 节点停止操作失败错误信息
func (e *StopError) Error() string {
	return fmt.Sprintf("server: %v, services: %v", e.Server, e.Services)
}
