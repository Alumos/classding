package app

import (
	"net/http"
	"regexp"
	"strings"
	"unicode"
)

type siteSettings struct {
	SiteName  string `json:"site_name"`
	ICPNumber string `json:"icp_number"`
	Theme     string `json:"theme"`
}

// authEpochKey holds a random value that changes on every restore; access
// tokens carry it, so a restore signs everyone out.
const authEpochKey = "_auth_epoch"

var icpNumberPattern = regexp.MustCompile(`^[\p{Han}]{1,4}ICP(?:备|证)[0-9]{4,20}号(?:-[0-9]{1,4})?$`)

// initializeSettings fills in defaults for settings that were never saved.
func initializeSettings(state *State) {
	if state.Settings == nil {
		state.Settings = map[string]string{}
	}
	for key, value := range map[string]string{"site_name": "课堂叮咚", "icp_number": "", "theme": "green"} {
		if _, exists := state.Settings[key]; !exists {
			state.Settings[key] = value
		}
	}
}

func (a *App) settings() siteSettings {
	return siteSettings{SiteName: a.state.Settings["site_name"], ICPNumber: a.state.Settings["icp_number"], Theme: a.state.Settings["theme"]}
}

func validSettings(s siteSettings) bool {
	if s.SiteName != strings.TrimSpace(s.SiteName) || !runesBetween(s.SiteName, 1, 60) || strings.ContainsFunc(s.SiteName, unicode.IsControl) {
		return false
	}
	if s.ICPNumber != "" && !icpNumberPattern.MatchString(s.ICPNumber) {
		return false
	}
	return s.Theme == "green" || s.Theme == "blue" || s.Theme == "slate"
}

func (a *App) getSettings(_ http.ResponseWriter, _ *http.Request, _ *User) (any, error) {
	return a.settings(), nil
}

func (a *App) updateSettings(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	var in struct {
		SiteName  *string `json:"site_name"`
		ICPNumber *string `json:"icp_number"`
		Theme     *string `json:"theme"`
	}
	if err := decodeStrict(r, &in); err != nil {
		return nil, err
	}
	if in.SiteName == nil || in.ICPNumber == nil || in.Theme == nil {
		return nil, bad("请完整填写站点名称、备案号和主题。")
	}
	s := siteSettings{SiteName: *in.SiteName, ICPNumber: *in.ICPNumber, Theme: *in.Theme}
	if !validSettings(s) {
		return nil, bad("站点名称、ICP备案号或主题格式不正确。")
	}
	a.state.Settings["site_name"] = s.SiteName
	a.state.Settings["icp_number"] = s.ICPNumber
	a.state.Settings["theme"] = s.Theme
	a.audit(u, "admin.settings.update", "site")
	return s, nil
}
