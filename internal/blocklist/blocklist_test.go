package blocklist

import (
	"testing"

	"github.com/shivang-16/orbit.api/internal/model"
)

func TestEmailBlocked(t *testing.T) {
	SetDomains([]string{"shit.ralsei.lol", "beetleai.dev", "uberip.com"})
	t.Cleanup(func() { SetDomains(nil) })

	cases := []struct {
		email   string
		blocked bool
	}{
		{"orbit@shit.ralsei.lol", true},
		{"anyuser@shit.ralsei.lol", true},
		{"ORBIT@SHIT.RALSEI.LOL", true},
		{"bot@mail.shit.ralsei.lol", true},
		{"user@beetleai.dev", true},
		{"anyone@beetleai.dev", true},
		{"bot@mail.beetleai.dev", true},
		{"user@uberip.com", true},
		{"anyone@uberip.com", true},
		{"bot@mail.uberip.com", true},
		{"ok@tryorbit.cloud", false},
		{"not-an-email", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := EmailBlocked(tc.email); got != tc.blocked {
			t.Fatalf("EmailBlocked(%q) = %v, want %v", tc.email, got, tc.blocked)
		}
	}

	if !UserIsBlocked(&model.User{Email: "anyone@beetleai.dev"}) {
		t.Fatal("expected domain user to be blocked")
	}
	if UserIsBlocked(&model.User{Email: "ok@tryorbit.cloud"}) {
		t.Fatal("expected normal user not to be blocked")
	}
	if UserIsBlocked(nil) {
		t.Fatal("expected nil user not to be blocked")
	}
}
