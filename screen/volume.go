package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// volumeSnapshot is the system output volume before a notification raised it.
type volumeSnapshot struct {
	DeviceID string  `json:"device_id"`
	Volume   float64 `json:"volume"`
	Muted    bool    `json:"muted"`
}

// The snapshot is also written to disk, so that a crash mid-notification can
// still be undone on the next start.
func (a *App) volumeRecoveryFile() string { return filepath.Join(a.dataDir, "volume-recovery.json") }

// PrepareAudio remembers the current volume and sets the class volume (0–100).
func (a *App) PrepareAudio(volume int) error {
	a.audioMu.Lock()
	defer a.audioMu.Unlock()
	if a.original != nil {
		return errors.New("已有播报正在进行")
	}
	old, err := audioGet()
	if err != nil {
		return err
	}
	recovery, err := json.Marshal(old)
	if err != nil {
		return err
	}
	if err = atomicWrite(a.volumeRecoveryFile(), recovery); err != nil {
		return err
	}
	a.original = old
	level := float64(min(max(volume, 0), 100)) / 100
	if err = audioSet(&volumeSnapshot{DeviceID: old.DeviceID, Volume: level}); err != nil {
		if audioSet(old) == nil {
			_ = os.Remove(a.volumeRecoveryFile())
			a.original = nil
		}
		return err
	}
	return nil
}

// RestoreAudio puts back the volume saved by PrepareAudio, from memory or disk.
func (a *App) RestoreAudio() error {
	a.audioMu.Lock()
	defer a.audioMu.Unlock()
	old := a.original
	if old == nil {
		data, err := os.ReadFile(a.volumeRecoveryFile())
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		old = new(volumeSnapshot)
		if err := json.Unmarshal(data, old); err != nil {
			return err
		}
	}
	if err := audioSet(old); err != nil {
		return err
	}
	if err := os.Remove(a.volumeRecoveryFile()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	a.original = nil
	return nil
}
