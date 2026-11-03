package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
)

type ScreenDisplay struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Primary bool   `json:"primary"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	rect    displayRect
	work    displayRect
}

type DisplaySettings struct {
	Selected string          `json:"selected"`
	Displays []ScreenDisplay `json:"displays"`
}

// displayPreference is saved as display.json; an empty ID means the primary display.
type displayPreference struct {
	Selected string `json:"selected"`
}

func (a *App) displayFile() string { return filepath.Join(a.dataDir, "display.json") }

func (a *App) loadDisplayPreference() {
	var saved displayPreference
	if data, err := os.ReadFile(a.displayFile()); err == nil && json.Unmarshal(data, &saved) == nil {
		a.displayID = saved.Selected
	}
}

func (a *App) selectedDisplay() string {
	a.displayMu.Lock()
	defer a.displayMu.Unlock()
	return a.displayID
}

func (a *App) GetDisplaySettings() (DisplaySettings, error) {
	if err := a.ready(); err != nil {
		return DisplaySettings{}, err
	}
	displays, err := listDisplays()
	if err != nil {
		return DisplaySettings{}, err
	}
	return DisplaySettings{Selected: a.selectedDisplay(), Displays: displays}, nil
}

// SetSelectedDisplay chooses where notifications and homework appear.
func (a *App) SetSelectedDisplay(id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	if id != "" {
		displays, err := listDisplays()
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(displays, func(d ScreenDisplay) bool { return d.ID == id }) {
			return errors.New("显示器已断开，请重新选择")
		}
	}
	data, err := json.Marshal(displayPreference{Selected: id})
	if err != nil {
		return err
	}
	a.displayMu.Lock()
	defer a.displayMu.Unlock()
	if err = atomicWrite(a.displayFile(), data); err != nil {
		return err
	}
	a.displayID = id
	go a.rerenderHomework()
	return nil
}
