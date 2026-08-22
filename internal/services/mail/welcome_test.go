package mail

import (
	"strings"
	"testing"
)

func TestWelcomeCopyIsOrbit(t *testing.T) {
	if welcomeEmailSubject != "Welcome to Orbit! 🎉" {
		t.Fatalf("subject = %q", welcomeEmailSubject)
	}
	html := welcomeEmailHTML()
	text := welcomeEmailText()
	for _, body := range []string{html, text} {
		if !strings.Contains(body, "Orbit") {
			t.Fatal("welcome copy missing Orbit")
		}
		if strings.Contains(body, "Choppr") {
			t.Fatal("welcome copy still mentions Choppr")
		}
	}
	if !strings.Contains(html, demoURL) || !strings.Contains(text, demoURL) {
		t.Fatal("welcome copy missing demo url")
	}
}
