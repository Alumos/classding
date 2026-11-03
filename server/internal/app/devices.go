package app

import (
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const bindCodeTTL = 10 * time.Minute

func publicDevice(d Device, online bool) DeviceView {
	d.TokenHash, d.Sequence = "", 0
	return DeviceView{Device: d, Online: online}
}

func (a *App) deviceView(d Device) DeviceView { return publicDevice(d, a.hub.DeviceOnline(d.ID)) }

func (a *App) deviceViews(include func(Device) bool) []DeviceView {
	out := []DeviceView{}
	for _, d := range a.state.Devices {
		if include(d) {
			out = append(out, a.deviceView(d))
		}
	}
	return out
}

func (a *App) classDevices(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	c, err := a.getClass(u, chi.URLParam(r, "id"), false)
	if err != nil {
		return nil, err
	}
	return a.deviceViews(func(d Device) bool { return d.ClassID == c.ID }), nil
}

func (a *App) dropBindings(classID string) {
	for code, b := range a.bindings {
		if b.ClassID == classID {
			delete(a.bindings, code)
		}
	}
}

// bindCode issues the six digits a classroom screen enters to join the class.
// Each class has at most one valid code at a time.
func (a *App) bindCode(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	c, err := a.usableClass(u, chi.URLParam(r, "id"), true)
	if err != nil {
		return nil, err
	}
	if !a.limit("bind-code:"+u.ID, 10, time.Minute) {
		return nil, tooMany("绑定码获取过于频繁。")
	}
	if a.deviceCount(c.ID) >= c.MaxDevices {
		return nil, fail(409, "DEVICE_LIMIT_REACHED", "大屏名额已满，请先解绑旧设备。")
	}
	a.dropBindings(c.ID)
	code := sixDigits()
	for _, taken := a.bindings[code]; taken; _, taken = a.bindings[code] {
		code = sixDigits()
	}
	b := binding{ClassID: c.ID, Expires: after(bindCodeTTL)}
	a.bindings[code] = b
	return map[string]string{"code": code, "expires_at": b.Expires}, nil
}

// deviceSnapshot is what a screen needs to render its class from scratch.
func (a *App) deviceSnapshot(d Device, c Class) map[string]any {
	return map[string]any{
		"device":            a.deviceView(d),
		"class":             map[string]string{"id": c.ID, "name": c.Name},
		"volume":            c.Volume,
		"homeworks":         a.homeworkItems(c.ID),
		"homework_revision": c.HomeworkRevision,
	}
}

func (a *App) bindDevice(_ http.ResponseWriter, r *http.Request, _ *User) (any, error) {
	if !a.limit("bind:"+a.clientIP(r), 5, time.Minute) {
		return nil, tooMany("绑定尝试过于频繁，请一分钟后再试。")
	}
	var in struct {
		Code          string `json:"code"`
		Name          string `json:"name"`
		OS            string `json:"os"`
		ClientVersion string `json:"client_version"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	b, exists := a.bindings[in.Code]
	if !exists || elapsed(b.Expires) {
		return nil, bad("绑定码不存在、已使用或已过期。")
	}
	c, exists := a.state.Classes[b.ClassID]
	if !exists || !a.licensed(c) {
		return nil, licenseInactive()
	}
	if a.deviceCount(c.ID) >= c.MaxDevices {
		return nil, fail(409, "DEVICE_LIMIT_REACHED", "大屏名额已满。")
	}
	if !runesBetween(in.Name, 0, maxDeviceName) || len(in.OS) > 80 || len(in.ClientVersion) > 30 {
		return nil, bad("设备信息过长。")
	}
	if name := strings.TrimSpace(in.Name); name == "" || name == "教室大屏" {
		in.Name = c.Name + "智慧黑板"
	}
	token := random(32)
	d := Device{ID: id("d"), ClassID: c.ID, Name: in.Name, OS: in.OS, ClientVersion: in.ClientVersion, LastSeenAt: now(), TokenHash: hash(token)}
	a.state.Devices[d.ID] = d
	delete(a.bindings, in.Code)
	a.bindPending(d)
	a.audit(nil, "device.bind", d.ID)
	snapshot := a.deviceSnapshot(d, c)
	snapshot["device_token"] = token
	return snapshot, nil
}

// deviceFromToken finds the screen that holds the device token.
func (a *App) deviceFromToken(token string) (Device, error) {
	if token == "" {
		return Device{}, unauthorized("设备尚未绑定。")
	}
	tokenHash := hash(token)
	for _, d := range a.state.Devices {
		if d.TokenHash == tokenHash {
			return d, nil
		}
	}
	return Device{}, deviceRevoked("设备绑定已失效。")
}

// boundDevice authenticates a screen request and loads its class.
func (a *App) boundDevice(r *http.Request) (Device, Class, error) {
	d, err := a.deviceFromToken(bearerToken(r))
	if err != nil {
		return d, Class{}, err
	}
	c, exists := a.state.Classes[d.ClassID]
	if !exists {
		return d, c, deviceRevoked("班级已被删除。")
	}
	return d, c, nil
}

func (a *App) deviceMe(_ http.ResponseWriter, r *http.Request, _ *User) (any, error) {
	d, c, err := a.boundDevice(r)
	if err != nil {
		return nil, err
	}
	snapshot := a.deviceSnapshot(d, c)
	snapshot["sequence"] = d.Sequence
	return snapshot, nil
}

// managedDevice loads a screen whose class the account manages.
func (a *App) managedDevice(u *User, deviceID string) (Device, error) {
	d, exists := a.state.Devices[deviceID]
	if !exists {
		return d, missing()
	}
	_, err := a.getClass(u, d.ClassID, true)
	return d, err
}

func (a *App) renameDevice(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	d, err := a.managedDevice(u, chi.URLParam(r, "id"))
	if err != nil {
		return nil, err
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || !runesBetween(in.Name, 1, maxDeviceName) {
		return nil, bad("设备名称需为 1–60 字。")
	}
	d.Name = name
	a.state.Devices[d.ID] = d
	return a.deviceView(d), nil
}

func (a *App) removeDevice(_ http.ResponseWriter, r *http.Request, u *User) (any, error) {
	d, err := a.managedDevice(u, chi.URLParam(r, "id"))
	if err != nil {
		return nil, err
	}
	a.unbind(d)
	delete(a.state.Devices, d.ID)
	for key, m := range a.state.Messages {
		for i, t := range m.Targets {
			if t.DeviceID == d.ID && t.Status == DeliveryQueued {
				m.Targets[i].Status = DeliveryFailed
			}
		}
		a.state.Messages[key] = m
	}
	a.audit(u, "device.unbind", d.ID)
	return ok(), nil
}

// unbind tells the screen it was removed and disconnects it after the change is saved.
func (a *App) unbind(d Device) {
	a.deferJob(func() {
		a.hub.Device(d.ID, Frame{T: "unbind", D: map[string]any{}})
		a.hub.RemoveDevice(d.ID)
		a.mu.Lock()
		defer a.mu.Unlock()
		a.presence(d, false)
	})
}

// deviceHello is what a screen reports about itself when it connects.
type deviceHello struct {
	Token           string   `json:"token"`
	LastSeq         int64    `json:"last_seq"`
	PendingIDs      []string `json:"pending_ids"`
	ClientVersion   string   `json:"client_version"`
	OS              string   `json:"os"`
	LANIPs          []string `json:"lan_ips"`
	ServerURL       string   `json:"server_url"`
	AppliedRevision string   `json:"applied_revision"`
}

const maxReportedAddresses = 8

// record stores the report on the device. Every value is bounded because it
// comes straight from the classroom computer.
func (h deviceHello) record(d *Device, publicIP string) {
	if version := strings.TrimSpace(h.ClientVersion); version != "" && len(version) <= 32 {
		d.ClientVersion = version
	}
	if name := strings.TrimSpace(h.OS); name != "" && len(name) <= 32 {
		d.OS = name
	}
	d.LANIPs = []string{}
	for _, value := range h.LANIPs {
		if ip := net.ParseIP(strings.TrimSpace(value)).To4(); ip != nil && len(d.LANIPs) < maxReportedAddresses {
			d.LANIPs = append(d.LANIPs, ip.String())
		}
	}
	if validServerURL(h.ServerURL) {
		d.ServerURL = strings.TrimSuffix(h.ServerURL, "/")
	}
	if len(h.AppliedRevision) <= 128 {
		d.AppliedRevision = h.AppliedRevision
	}
	if ip := net.ParseIP(publicIP); ip != nil {
		d.PublicIP = ip.String()
	}
}

// validServerURL accepts only the root of an HTTP or HTTPS site.
func validServerURL(raw string) bool {
	if raw == "" || raw != strings.TrimSpace(raw) || len(raw) > 2048 || strings.ContainsAny(raw, "\\?#\r\n\t") {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.Scheme != "http" && u.Scheme != "https" || u.Hostname() == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery || u.RawPath != "" || u.Path != "" && u.Path != "/" {
		return false
	}
	if strings.ContainsAny(u.Hostname(), " %") || strings.HasPrefix(u.Host, "[") && net.ParseIP(u.Hostname()) == nil {
		return false
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		return err == nil && inRange(n, 1, 65535)
	}
	return !strings.HasSuffix(u.Host, ":")
}
