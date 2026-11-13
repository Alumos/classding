package app

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const releaseBase = "https://github.com/Alumos/classding/releases/download/v" + Version

func (a *App) routes() {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer, middleware.RequestID, securityHeaders)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok", "version": Version})
	})
	r.Get("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "classding_online_devices %d\n", a.hub.DeviceCount())
	})
	r.Get("/ws/device", a.deviceWS)
	r.Get("/ws/teacher", a.teacherWS)

	r.Route("/api/v1", func(r chi.Router) {
		q, c := a.query, a.command

		r.Get("/config", q(public, a.siteConfig))
		r.Get("/client/latest", q(public, clientRelease))

		r.Post("/auth/register", c(public, a.register))
		r.Post("/auth/login", c(public, a.login))
		r.Post("/auth/refresh", c(public, a.refresh))
		r.Post("/auth/logout", c(public, a.logout))
		r.Get("/me", q(signedIn, me))
		r.Patch("/me", c(signedIn, a.updateMe))
		r.Put("/me/password", c(signedIn, a.changePassword))

		r.Get("/classes", q(signedIn, a.listClasses))
		r.Post("/classes", c(signedIn, a.createClass))
		r.Post("/classes/join", c(signedIn, a.joinClass))
		r.Patch("/classes/{id}", c(signedIn, a.updateClass))
		r.Delete("/classes/{id}", c(signedIn, a.deleteClass))
		r.Get("/classes/{id}/members", q(signedIn, a.members))
		r.Post("/classes/{id}/invites", c(signedIn, a.invite))
		r.Delete("/classes/{id}/members/{uid}", c(signedIn, a.removeMember))
		r.Post("/classes/{id}/transfer", c(signedIn, a.transfer))
		r.Post("/classes/{id}/leave", c(signedIn, a.leave))
		r.Get("/classes/{id}/devices", q(signedIn, a.classDevices))
		r.Post("/classes/{id}/bind-code", c(signedIn, a.bindCode))
		r.Post("/classes/{id}/tools/{tool}", c(signedIn, a.tool))

		r.Post("/device/bind", c(public, a.bindDevice))
		r.Get("/device/me", q(public, a.deviceMe))
		r.Get("/device/homeworks", q(public, a.deviceHomeworks))
		r.Post("/device/student-calls", c(public, a.createStudentCall))
		r.Get("/student-calls", q(signedIn, a.listStudentCalls))
		r.Post("/student-calls/{id}/read", c(signedIn, a.readStudentCall))
		r.Post("/device/migration-check", q(public, a.deviceMigrationCheck))
		r.Patch("/devices/{id}", c(signedIn, a.renameDevice))
		r.Delete("/devices/{id}", c(signedIn, a.removeDevice))

		r.Get("/homeworks", q(signedIn, a.listHomeworks))
		r.Post("/homeworks", c(signedIn, a.createHomework))
		r.Patch("/homeworks/{id}", c(signedIn, a.updateHomework))
		r.Delete("/homeworks/{id}", c(signedIn, a.deleteHomework))

		r.Get("/messages", q(signedIn, a.listMessages))
		r.Post("/messages", c(signedIn, a.sendMessage))
		r.Get("/messages/{id}", q(signedIn, a.getMessage))
		r.Delete("/messages/{id}", c(signedIn, a.deleteMessage))
		r.Post("/messages/{id}/recall", c(signedIn, a.recallMessage))
		r.Post("/messages/{id}/resend", c(signedIn, a.resendMessage))
		r.Get("/phrases", q(signedIn, a.listPhrases))
		r.Post("/phrases", c(signedIn, a.addPhrase))
		r.Patch("/phrases/{id}", c(signedIn, a.editPhrase))
		r.Delete("/phrases/{id}", c(signedIn, a.deletePhrase))
		r.Post("/audio", c(signedIn, a.uploadAudio))
		r.Get("/audio/{id}", a.downloadAudio)

		r.Get("/admin/overview", q(adminOnly, a.overview))
		r.Get("/admin/activation-codes", q(adminOnly, a.listCodes))
		r.Post("/admin/activation-codes", c(superAdminOnly, a.createCodes))
		r.Patch("/admin/activation-codes/{code}", c(superAdminOnly, a.updateCode))
		r.Post("/admin/activation-codes/delete", c(superAdminOnly, a.deleteCodes))
		r.Get("/admin/schools", q(adminOnly, a.listSchools))
		r.Post("/admin/schools", c(superAdminOnly, a.createSchool))
		r.Patch("/admin/schools/{id}", c(superAdminOnly, a.updateSchool))
		r.Delete("/admin/schools/{id}", c(superAdminOnly, a.deleteSchool))
		r.Get("/admin/users", q(adminOnly, a.adminUsers))
		r.Post("/admin/users", c(adminOnly, a.adminCreateUser))
		r.Patch("/admin/users/{id}", c(adminOnly, a.adminUpdateUser))
		r.Delete("/admin/users/{id}", c(adminOnly, a.adminDeleteUser))
		r.Post("/admin/users/{id}/unlock-login", c(adminOnly, a.adminUnlockLogin))
		r.Get("/admin/classes", q(adminOnly, a.adminClasses))
		r.Post("/admin/classes", c(adminOnly, a.adminCreateClass))
		r.Patch("/admin/classes/{id}", c(adminOnly, a.adminUpdateClass))
		r.Delete("/admin/classes/{id}", c(adminOnly, a.adminDeleteClass))
		r.Get("/admin/devices", q(adminOnly, a.adminDevices))
		r.Post("/admin/devices/{id}/config", c(superAdminOnly, a.configureDevice))
		r.Get("/admin/messages", c(adminOnly, a.adminMessages)) // Reading is audited.
		r.Delete("/admin/messages/{id}", c(adminOnly, a.adminDeleteMessage))
		r.Get("/admin/audit-logs", q(superAdminOnly, a.auditLogs))
		r.Get("/admin/settings", q(superAdminOnly, a.getSettings))
		r.Put("/admin/settings", c(superAdminOnly, a.updateSettings))
		r.Get("/admin/backup", a.backup)
		r.Post("/admin/restore", a.restore)
	})
	a.router = r
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func me(_ http.ResponseWriter, _ *http.Request, u *User) (any, error) { return u.Public(), nil }

func (a *App) siteConfig(_ http.ResponseWriter, _ *http.Request, _ *User) (any, error) {
	s := a.settings()
	return map[string]any{
		"version":              Version,
		"speech_enabled":       false,
		"registration_enabled": true,
		"site_name":            s.SiteName,
		"icp_number":           s.ICPNumber,
		"theme":                s.Theme,
	}, nil
}

func clientRelease(_ http.ResponseWriter, _ *http.Request, _ *User) (any, error) {
	return map[string]string{
		"version":       Version,
		"url":           releaseBase + "/ClassDingScreen-amd64-installer.exe",
		"sha256":        "",
		"notes":         "支持只显示不朗读的通知（朗读 0 遍）；SHA-256 校验文件见对应 GitHub Release。",
		"min_supported": "0.1.0",
	}, nil
}
