//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

type dataBlob struct {
	Size uint32
	Data *byte
}

var crypt32 = syscall.NewLazyDLL("crypt32.dll")
var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var cryptProtectData = crypt32.NewProc("CryptProtectData")
var cryptUnprotectData = crypt32.NewProc("CryptUnprotectData")
var localFree = kernel32.NewProc("LocalFree")

func dpapi(input []byte, protectData bool) ([]byte, error) {
	var in dataBlob
	in.Size = uint32(len(input))
	if len(input) > 0 {
		in.Data = &input[0]
	}
	var out dataBlob
	var ok uintptr
	var callErr error
	if protectData {
		ok, _, callErr = cryptProtectData.Call(uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0, 1, uintptr(unsafe.Pointer(&out)))
	} else {
		ok, _, callErr = cryptUnprotectData.Call(uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0, 1, uintptr(unsafe.Pointer(&out)))
	}
	if ok == 0 {
		return nil, fmt.Errorf("Windows DPAPI: %w", callErr)
	}
	defer localFree.Call(uintptr(unsafe.Pointer(out.Data)))
	if out.Size == 0 {
		return []byte{}, nil
	}
	return append([]byte(nil), unsafe.Slice(out.Data, int(out.Size))...), nil
}
func protect(data []byte) ([]byte, error)   { return dpapi(data, true) }
func unprotect(data []byte) ([]byte, error) { return dpapi(data, false) }
