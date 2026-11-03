package app

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
)

type siteSettings struct {
	SiteName  string `json:"site_name"`
	ICPNumber string `json:"icp_number"`
	Theme     string `json:"theme"`
}

var icpNumberPattern = regexp.MustCompile(`^[\p{Han}]{1,4}ICP(?:备|证)[0-9]{4,20}号(?:-[0-9]{1,4})?$`)

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
	if s.SiteName != strings.TrimSpace(s.SiteName) || utf8.RuneCountInString(s.SiteName) < 1 || utf8.RuneCountInString(s.SiteName) > 60 || strings.IndexFunc(s.SiteName, unicode.IsControl) >= 0 {
		return false
	}
	if s.ICPNumber != "" && !icpNumberPattern.MatchString(s.ICPNumber) {
		return false
	}
	return s.Theme == "green" || s.Theme == "blue" || s.Theme == "slate"
}

func decodeStrict(r *http.Request, dst any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return bad("请求格式不正确。")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return bad("请求包含多余内容。")
	}
	return nil
}

func (a *App) getSettings(_ http.ResponseWriter, _ *http.Request, u *User) (any, error) {
	if u.Role != "super_admin" {
		return nil, forbidden()
	}
	return a.settings(), nil
}

func (a *App) updateSettings(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	if u.Role != "super_admin" {
		return nil, forbidden()
	}
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

func validServerURL(raw string) bool {
	if raw == "" || raw != strings.TrimSpace(raw) || len(raw) > 2048 || strings.ContainsAny(raw, "\\?#\r\n\t") {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery || u.RawPath != "" || (u.Path != "" && u.Path != "/") {
		return false
	}
	if strings.ContainsAny(u.Hostname(), " %") {
		return false
	}
	if strings.HasPrefix(u.Host, "[") && net.ParseIP(u.Hostname()) == nil {
		return false
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
	} else if strings.HasSuffix(u.Host, ":") {
		return false
	}
	return true
}

func (a *App) configureDevice(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	if u.Role != "super_admin" {
		return nil, forbidden()
	}
	d, exists := a.state.Devices[chi.URLParam(r, "id")]
	if !exists {
		return nil, missing()
	}
	var in struct {
		ServerURL *string `json:"server_url"`
	}
	if err := decodeStrict(r, &in); err != nil {
		return nil, err
	}
	if in.ServerURL == nil || (*in.ServerURL != "" && !validServerURL(*in.ServerURL)) {
		return nil, bad("迁移地址必须是 HTTP 或 HTTPS 站点根地址。")
	}
	if *in.ServerURL == "" {
		d.RemoteConfig = nil
	} else {
		d.RemoteConfig = &RemoteConfig{ServerURL: strings.TrimSuffix(*in.ServerURL, "/"), Revision: id("rev")}
	}
	a.state.Devices[d.ID] = d
	a.audit(u, "admin.device.config", d.ID)
	a.deferJob(func() { a.hub.Device(d.ID, Frame{T: "config", D: map[string]any{"remote_config": d.RemoteConfig}}) })
	d.Online = a.hub.DeviceOnline(d.ID)
	return publicDevice(d), nil
}
