//go:build !windows

package main

type trayHandle struct{}

func supportsTray() bool                            { return false }
func startTray(_, _, _ func()) (*trayHandle, error) { return nil, nil }
func (*trayHandle) close()                          {}
