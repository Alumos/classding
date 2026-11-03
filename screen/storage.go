package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const maxStateBytes = 8 << 20

// LoadState returns the frontend's saved state, decrypted, or "" on first run.
func (a *App) LoadState() (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(a.dataDir, "state.bin"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	plain, err := unprotect(data)
	if err != nil {
		return "", fmt.Errorf("无法读取本机加密数据：%w", err)
	}
	return string(plain), nil
}

// SaveState encrypts the frontend's state for this Windows account and stores it.
func (a *App) SaveState(value string) error {
	if err := a.ready(); err != nil {
		return err
	}
	if len(value) > maxStateBytes {
		return errors.New("本地状态过大")
	}
	data, err := protect([]byte(value))
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(a.dataDir, "state.bin"), data)
}

// atomicWrite replaces path so that readers see either the old or the new file.
func atomicWrite(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".classding-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err = temp.Chmod(0600); err == nil {
		_, err = temp.Write(data)
	}
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}
