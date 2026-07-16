package auth

import "testing"

func TestPasswordHash(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(hash, "correct horse battery staple") {
		t.Fatal("password should match")
	}
	if CheckPassword(hash, "wrong password") {
		t.Fatal("wrong password matched")
	}
}

func TestPasswordMinimumLength(t *testing.T) {
	if _, err := HashPassword("too-short"); err == nil {
		t.Fatal("expected minimum length error")
	}
}
