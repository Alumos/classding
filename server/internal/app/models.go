package app

import "time"

// Version is the release reported by the server and offered to classroom clients.
const Version = "0.4.0"

// Role decides which workspace an account may sign in to and what it governs.
type Role string

const (
	RoleTeacher     Role = "user"
	RoleSchoolAdmin Role = "school_admin"
	RoleSuperAdmin  Role = "super_admin"
)

type AccountStatus string

const (
	AccountActive   AccountStatus = "active"
	AccountDisabled AccountStatus = "disabled"
)

// MemberRole is a teacher's position within one class.
type MemberRole string

const (
	MemberOwner        MemberRole = "owner"
	MemberCollaborator MemberRole = "collaborator"
)

type LicenseStatus string

const (
	LicenseUnused   LicenseStatus = "unused"
	LicenseActive   LicenseStatus = "active"
	LicenseRevoked  LicenseStatus = "revoked"
	LicenseReleased LicenseStatus = "released"
)

// DeliveryStatus follows one notification on one classroom screen.
type DeliveryStatus string

const (
	DeliveryQueued    DeliveryStatus = "queued"
	DeliveryDelivered DeliveryStatus = "delivered"
	DeliveryPlaying   DeliveryStatus = "playing"
	DeliveryPlayed    DeliveryStatus = "played"
	DeliveryDismissed DeliveryStatus = "dismissed"
	DeliveryFailed    DeliveryStatus = "failed"
	DeliveryExpired   DeliveryStatus = "expired"
)

// Pending reports whether the screen may still show the notification.
func (s DeliveryStatus) Pending() bool {
	return s == DeliveryQueued || s == DeliveryDelivered || s == DeliveryPlaying
}

// progress orders statuses so that acknowledgements only ever move forward.
func (s DeliveryStatus) progress() int {
	switch s {
	case DeliveryQueued:
		return 0
	case DeliveryDelivered:
		return 1
	case DeliveryPlaying:
		return 2
	default:
		return 3
	}
}

type User struct {
	ID               string        `json:"id"`
	Username         string        `json:"username"`
	DisplayName      string        `json:"display_name"`
	Role             Role          `json:"role"`
	OrgID            string        `json:"org_id"`
	Status           AccountStatus `json:"status"`
	PasswordHash     string        `json:"password_hash,omitempty"`
	TokenVersion     int           `json:"token_version,omitempty"`
	LoginLockedUntil string        `json:"login_locked_until,omitempty"`
	UIScale          int           `json:"ui_scale,omitempty"` // Interface text size in percent; 0 means the default.
}

func (u User) Active() bool       { return u.Status == AccountActive }
func (u User) IsAdmin() bool      { return u.Role == RoleSchoolAdmin || u.Role == RoleSuperAdmin }
func (u User) IsSuperAdmin() bool { return u.Role == RoleSuperAdmin }

// Governs reports whether an administrator manages records of the organization.
func (u User) Governs(orgID string) bool {
	return u.Role == RoleSuperAdmin || u.Role == RoleSchoolAdmin && u.OrgID != "" && u.OrgID == orgID
}

// Public strips credentials before the account leaves the server.
func (u User) Public() User {
	u.PasswordHash, u.TokenVersion = "", 0
	return u
}

type Class struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Grade            string `json:"grade"`
	OwnerID          string `json:"owner_id"`
	OrgID            string `json:"org_id"`
	Volume           int    `json:"volume"`
	MaxTeachers      int    `json:"max_teachers"`
	MaxDevices       int    `json:"max_devices"`
	CreatedAt        string `json:"created_at"`
	ActivationCode   string `json:"activation_code,omitempty"`
	HomeworkRevision int64  `json:"homework_revision,omitempty"`
}

// ClassView is a class as one account sees it, with live usage counts.
type ClassView struct {
	Class
	Role          MemberRole `json:"role"`
	TeachersCount int        `json:"teachers_count"`
	DevicesCount  int        `json:"devices_count"`
	OnlineDevices int        `json:"online_devices"`
}

type Homework struct {
	ID          string `json:"id"`
	ClassID     string `json:"class_id"`
	Subject     string `json:"subject"`
	Text        string `json:"text"`
	TeacherID   string `json:"teacher_id"`
	TeacherName string `json:"teacher_name"`
	Visible     bool   `json:"visible"`
	AlwaysOnTop bool   `json:"always_on_top"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type Member struct {
	UserID      string     `json:"user_id"`
	ClassID     string     `json:"class_id"`
	DisplayName string     `json:"display_name"`
	Username    string     `json:"username"`
	Role        MemberRole `json:"role"`
	JoinedAt    string     `json:"joined_at"`
}

// memberKey is the primary key of a class membership.
func memberKey(classID, userID string) string { return classID + "|" + userID }

type Device struct {
	ID            string        `json:"id"`
	ClassID       string        `json:"class_id"`
	Name          string        `json:"name"`
	OS            string        `json:"os"`
	ClientVersion string        `json:"client_version"`
	LastSeenAt    string        `json:"last_seen_at"`
	TokenHash     string        `json:"token_hash,omitempty"`
	Sequence      int64         `json:"sequence,omitempty"`
	RemoteConfig  *RemoteConfig `json:"remote_config,omitempty"`
	// Reported by the client on each connection.
	LANIPs          []string `json:"lan_ips,omitempty"`
	PublicIP        string   `json:"public_ip,omitempty"`
	ServerURL       string   `json:"server_url,omitempty"`
	AppliedRevision string   `json:"applied_revision,omitempty"`
}

// DeviceView is a classroom screen without its credentials, with live presence.
type DeviceView struct {
	Device
	Online bool `json:"online"`
}

// RemoteConfig tells a classroom screen to move to another server.
type RemoteConfig struct {
	ServerURL string `json:"server_url"`
	Revision  string `json:"revision"`
}

type Target struct {
	ClassID     string         `json:"class_id"`
	ClassName   string         `json:"class_name"`
	DeviceID    string         `json:"device_id"`
	DeviceName  string         `json:"device_name"`
	Status      DeliveryStatus `json:"status"`
	Seq         int64          `json:"seq"`
	DeliveredAt *string        `json:"delivered_at"`
	PlayedAt    *string        `json:"played_at"`
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

// Live reports whether the notification may still reach a screen.
func (m Message) Live() bool { return m.RecalledAt == nil && !elapsed(m.ExpiresAt) }

// ClassIDs lists the target classes in their original order.
func (m Message) ClassIDs() []string {
	ids := make([]string, len(m.Targets))
	for i, t := range m.Targets {
		ids[i] = t.ClassID
	}
	return unique(ids)
}

// unique drops repeated values and keeps the first occurrence of each.
func unique(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

// Code is an activation license; redeeming it creates one class.
type Code struct {
	Code        string        `json:"code"`
	MaxTeachers int           `json:"max_teachers"`
	MaxDevices  int           `json:"max_devices"`
	ExpiresAt   *string       `json:"expires_at"`
	Status      LicenseStatus `json:"status"`
	ClassID     *string       `json:"class_id"`
	OrgID       string        `json:"org_id"`
	BatchID     string        `json:"batch_id"`
	Note        string        `json:"note"`
	RedeemedAt  *string       `json:"redeemed_at"`
}

func (c Code) Expired() bool { return c.ExpiresAt != nil && elapsed(*c.ExpiresAt) }

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

// School is an organization with a name; see schools.go.
type School struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

type Audit struct {
	ID     string `json:"id"`
	Actor  string `json:"actor"`
	Action string `json:"action"`
	Target string `json:"target"`
	At     string `json:"at"`
}

// StudentCall is a shared classroom request. Names are immutable event snapshots.
// Read fields deliberately have no omitempty: unread values are empty strings.
type StudentCall struct {
	ID         string `json:"id"`
	ClassID    string `json:"class_id"`
	ClassName  string `json:"class_name"`
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	CreatedAt  string `json:"created_at"`
	ReadAt     string `json:"read_at"`
	ReadBy     string `json:"read_by"`
	ReadByName string `json:"read_by_name"`
}

// studentCallRecord keeps replay keys in storage/backups, never in API responses.
type studentCallRecord struct {
	StudentCall
	RequestID string `json:"request_id"`
}

// State is the whole database. It lives in memory under App.mu and is written
// through to SQLite after every successful change.
type State struct {
	StudentCalls map[string]studentCallRecord `json:"student_calls"`
	Users        map[string]User              `json:"users"`
	Classes      map[string]Class             `json:"classes"`
	Members      map[string]Member            `json:"class_members"`
	Devices      map[string]Device            `json:"devices"`
	Messages     map[string]Message           `json:"messages"`
	Homeworks    map[string]Homework          `json:"homeworks"`
	Codes        map[string]Code              `json:"activation_codes"`
	Invites      map[string]Invite            `json:"collab_invites"`
	Sessions     map[string]Session           `json:"sessions"`
	Phrases      map[string]Phrase            `json:"phrases"`
	Audio        map[string]Audio             `json:"audio"`
	Audit        map[string]Audit             `json:"audit_logs"`
	Settings     map[string]string            `json:"settings"`
	Schools      map[string]School            `json:"schools"`
}

func newState() State {
	return State{
		StudentCalls: map[string]studentCallRecord{},
		Users:        map[string]User{},
		Classes:      map[string]Class{},
		Members:      map[string]Member{},
		Devices:      map[string]Device{},
		Messages:     map[string]Message{},
		Homeworks:    map[string]Homework{},
		Codes:        map[string]Code{},
		Invites:      map[string]Invite{},
		Sessions:     map[string]Session{},
		Phrases:      map[string]Phrase{},
		Audio:        map[string]Audio{},
		Audit:        map[string]Audit{},
		Settings:     map[string]string{},
		Schools:      map[string]School{},
	}
}

// Timestamps are stored as RFC 3339 strings in UTC so they sort lexically.
func now() string                         { return stamp(time.Now()) }
func after(duration time.Duration) string { return stamp(time.Now().Add(duration)) }
func stamp(at time.Time) string           { return at.UTC().Format(time.RFC3339Nano) }

func parseTime(value string) (time.Time, bool) {
	at, err := time.Parse(time.RFC3339Nano, value)
	return at, err == nil
}

// elapsed treats an unreadable timestamp as already past.
func elapsed(value string) bool {
	at, ok := parseTime(value)
	return !ok || time.Now().After(at)
}
