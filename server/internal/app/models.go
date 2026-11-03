package app

import "time"

type User struct {
	ID               string `json:"id"`
	Username         string `json:"username"`
	DisplayName      string `json:"display_name"`
	Role             string `json:"role"`
	OrgID            string `json:"org_id"`
	Status           string `json:"status"`
	PasswordHash     string `json:"password_hash,omitempty"`
	TokenVersion     int    `json:"token_version,omitempty"`
	LoginLockedUntil string `json:"login_locked_until,omitempty"`
}
type Class struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Grade          string `json:"grade"`
	OwnerID        string `json:"owner_id"`
	OrgID          string `json:"org_id"`
	Volume         int    `json:"volume"`
	Role           string `json:"role"`
	MaxTeachers    int    `json:"max_teachers"`
	MaxDevices     int    `json:"max_devices"`
	TeachersCount  int    `json:"teachers_count"`
	DevicesCount   int    `json:"devices_count"`
	OnlineDevices  int    `json:"online_devices"`
	CreatedAt      string `json:"created_at"`
	ActivationCode string `json:"activation_code,omitempty"`
}
type Member struct {
	UserID      string `json:"user_id"`
	ClassID     string `json:"class_id"`
	DisplayName string `json:"display_name"`
	Username    string `json:"username"`
	Role        string `json:"role"`
	JoinedAt    string `json:"joined_at"`
}
type Device struct {
	ID            string `json:"id"`
	ClassID       string `json:"class_id"`
	Name          string `json:"name"`
	OS            string `json:"os"`
	ClientVersion string `json:"client_version"`
	Online        bool   `json:"online"`
	LastSeenAt    string `json:"last_seen_at"`
	TokenHash     string `json:"token_hash,omitempty"`
	Sequence      int64  `json:"sequence,omitempty"`
}
type Target struct {
	ClassID     string  `json:"class_id"`
	ClassName   string  `json:"class_name"`
	DeviceID    string  `json:"device_id"`
	DeviceName  string  `json:"device_name"`
	Status      string  `json:"status"`
	Seq         int64   `json:"seq"`
	DeliveredAt *string `json:"delivered_at"`
	PlayedAt    *string `json:"played_at"`
}
type Message struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Text        string   `json:"text"`
	StudentName string   `json:"student_name"`
	Signature   string   `json:"signature"`
	Style       string   `json:"style"`
	DisplaySec  int      `json:"display_sec"`
	Repeat      int      `json:"repeat"`
	Chime       bool     `json:"chime"`
	PlayMode    string   `json:"play_mode"`
	SenderID    string   `json:"sender_id"`
	CreatedAt   string   `json:"created_at"`
	ExpiresAt   string   `json:"expires_at"`
	RecalledAt  *string  `json:"recalled_at"`
	Targets     []Target `json:"targets"`
	AudioID     string   `json:"audio_id,omitempty"`
}
type Code struct {
	Code        string  `json:"code"`
	MaxTeachers int     `json:"max_teachers"`
	MaxDevices  int     `json:"max_devices"`
	ExpiresAt   *string `json:"expires_at"`
	Status      string  `json:"status"`
	ClassID     *string `json:"class_id"`
	OrgID       string  `json:"org_id"`
	BatchID     string  `json:"batch_id"`
	Note        string  `json:"note"`
	RedeemedAt  *string `json:"redeemed_at"`
}
type Invite struct {
	Code      string `json:"code"`
	ClassID   string `json:"class_id"`
	ExpiresAt string `json:"expires_at"`
	Used      bool   `json:"used"`
}
type Session struct {
	ID            string `json:"id"`
	UserID        string `json:"user_id"`
	ExpiresAt     string `json:"expires_at"`
	Version       int    `json:"version"`
	AdminVerified bool   `json:"admin_verified,omitempty"` // Legacy field; no longer used for authentication.
	RotationUntil string `json:"rotation_until,omitempty"`
	Scope         string `json:"auth_scope,omitempty"`
}
type Phrase struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Text   string `json:"text"`
	Sort   int    `json:"sort"`
}
type Audio struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Path      string `json:"path"`
	MIME      string `json:"mime"`
	CreatedAt string `json:"created_at"`
}
type Audit struct {
	ID     string `json:"id"`
	Actor  string `json:"actor"`
	Action string `json:"action"`
	Target string `json:"target"`
	At     string `json:"at"`
}
type State struct {
	Users    map[string]User    `json:"users"`
	Classes  map[string]Class   `json:"classes"`
	Members  map[string]Member  `json:"class_members"`
	Devices  map[string]Device  `json:"devices"`
	Messages map[string]Message `json:"messages"`
	Codes    map[string]Code    `json:"activation_codes"`
	Invites  map[string]Invite  `json:"collab_invites"`
	Sessions map[string]Session `json:"sessions"`
	Phrases  map[string]Phrase  `json:"phrases"`
	Audio    map[string]Audio   `json:"audio"`
	Audit    map[string]Audit   `json:"audit_logs"`
}

func newState() State {
	return State{map[string]User{}, map[string]Class{}, map[string]Member{}, map[string]Device{}, map[string]Message{}, map[string]Code{}, map[string]Invite{}, map[string]Session{}, map[string]Phrase{}, map[string]Audio{}, map[string]Audit{}}
}
func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func after(duration time.Duration) string {
	return time.Now().UTC().Add(duration).Format(time.RFC3339Nano)
}
func elapsed(at string) bool {
	parsed, err := time.Parse(time.RFC3339Nano, at)
	return err != nil || time.Now().After(parsed)
}
func publicUser(u User) User       { u.PasswordHash = ""; u.TokenVersion = 0; return u }
func publicDevice(d Device) Device { d.TokenHash = ""; d.Sequence = 0; return d }
