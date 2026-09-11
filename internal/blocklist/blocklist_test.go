package blocklist

import "testing"

func TestEmailBlocked(t *testing.T) {
	t.Parallel()
	cases := []struct {
		email   string
		blocked bool
	}{
		{"orbit@shit.ralsei.lol", true},
		{"ORBIT@SHIT.RALSEI.LOL", true},
		{"bot@mail.shit.ralsei.lol", true},
		{"ok@tryorbit.cloud", false},
		{"not-an-email", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := EmailBlocked(tc.email); got != tc.blocked {
			t.Fatalf("EmailBlocked(%q) = %v, want %v", tc.email, got, tc.blocked)
		}
	}
}
