package persistence

import (
	"strings"
	"testing"
)

func TestValidateAccountCredentials(t *testing.T) {
	if err := validateAccountCredentials("player1", "123456"); err != nil {
		t.Fatalf("valid credentials: %v", err)
	}
	if err := validateAccountCredentials("", "123456"); err != ErrNicknameInvalid {
		t.Fatalf("empty nickname: %v", err)
	}
	if err := validateAccountCredentials(strings.Repeat("a", 65), "123456"); err != ErrNicknameInvalid {
		t.Fatalf("long nickname: %v", err)
	}
	if err := validateAccountCredentials("p", "12345"); err != ErrPasswordInvalid {
		t.Fatalf("short password: %v", err)
	}
	if err := validateAccountCredentials("p", strings.Repeat("x", 73)); err != ErrPasswordInvalid {
		t.Fatalf("long password: %v", err)
	}
}
