//go:build !windows

package main

import "errors"

func audioGet() (*volumeSnapshot, error) {
	return nil, errors.New("系统音量控制仅支持 Windows")
}
func audioSet(_ *volumeSnapshot) error { return errors.New("系统音量控制仅支持 Windows") }
func setAutostart(_ bool) error        { return errors.New("开机启动仅支持 Windows") }
func getAutostart() (bool, error)      { return false, nil }
