//go:build !windows

package main

import "errors"

type displayRect struct{}

func listDisplays() ([]ScreenDisplay, error) {
	return nil, errors.New("显示器选择仅支持 Windows")
}
func moveToDisplay(_ string) error { return nil }
