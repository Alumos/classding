package app

import (
	"strings"
	"testing"
)

func TestDeviceSequenceSurvivesHistoryCleanupAndRestart(t *testing.T) {
	config := Config{DataDir: t.TempDir(), JWTSecret: strings.Repeat("k", 40)}
	withApp := func(work func(*App)) {
		t.Helper()
		service, err := New(config)
		if err != nil {
			t.Fatal(err)
		}
		defer service.Close()
		work(service)
		if err = service.Save(); err != nil {
			t.Fatal(err)
		}
	}
	// A legacy database only stores sequence numbers on message targets.
	withApp(func(service *App) {
		service.state.Devices["device"] = Device{ID: "device"}
		service.state.Messages["legacy"] = Message{ID: "legacy", Targets: []Target{{DeviceID: "device", Seq: 57}}}
	})
	withApp(func(service *App) {
		if service.state.Devices["device"].Sequence != 57 {
			t.Fatal("legacy sequence was not migrated")
		}
		delete(service.state.Messages, "legacy")
		if next := service.nextSeq("device"); next != 58 {
			t.Fatalf("history cleanup reset sequence: %d", next)
		}
	})
	withApp(func(service *App) {
		if next := service.nextSeq("device"); next != 59 {
			t.Fatalf("restart reset sequence: %d", next)
		}
		if publicDevice(service.state.Devices["device"]).Sequence != 0 {
			t.Fatal("internal counter exposed")
		}
	})
}

func TestBootstrapWithoutTOTPAndNormalizesUsername(t *testing.T) {
	service, err := New(Config{DataDir: t.TempDir(), JWTSecret: strings.Repeat("k", 40)})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if err := service.BootstrapAdmin(" Operator ", "AdminPassword2026!", "管理员"); err != nil {
		t.Fatal(err)
	}
	if err := service.BootstrapAdmin("OPERATOR", "DifferentPassword2026!", "管理员"); err != nil {
		t.Fatal(err)
	}
	if len(service.state.Users) != 1 {
		t.Fatal("bootstrap created duplicate administrator")
	}
	for _, user := range service.state.Users {
		if user.Username != "operator" || !passwordMatches("AdminPassword2026!", user.PasswordHash) {
			t.Fatal("bootstrap changed existing credentials")
		}
	}
	service.state.Users["teacher"] = User{ID: "teacher", Username: "teacher", Role: "user"}
	if err := service.BootstrapAdmin("TEACHER", "AdminPassword2026!", "管理员"); err == nil {
		t.Fatal("bootstrap accepted a teacher identity")
	}
}
