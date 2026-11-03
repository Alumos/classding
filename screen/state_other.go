//go:build !windows

package main

// Non-Windows builds are for development; production Windows builds use DPAPI.
func protect(data []byte) ([]byte, error)   { return data, nil }
func unprotect(data []byte) ([]byte, error) { return data, nil }
