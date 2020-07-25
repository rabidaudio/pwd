package main

import (
	"bytes"
	"crypto/sha1"
	"crypto/subtle"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/crypto/pbkdf2"
)

func test(keySize int) bool {
	saltLen := keySize / 2
	saltpwvv := make([]byte, saltLen+2)
	salt := saltpwvv[:saltLen]
	pwvv := saltpwvv[saltLen : saltLen+2]
	password := []byte("password")
	totalSize := (keySize * 2) + 2
	key := pbkdf2.Key(password, salt, 1000, totalSize, sha1.New)
	// encKey = key[:keySize]
	// authKey = key[keySize : keySize*2]
	pwv := key[keySize*2:]
	b := subtle.ConstantTimeCompare(pwvv, pwv) > 0
	return b
}

// ~1ms single-threaded
func BenchmarkPasswordVerificationRateAES256(b *testing.B) {
	for i := 0; i < b.N; i++ {
		test(16)
	}
}

// ~.33ms parallel
func BenchmarkPasswordVerificationRateParallelAES256(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			test(16)
		}
	})
}

// ~0.5ms single-threaded
func BenchmarkPasswordVerificationRateAES128(b *testing.B) {
	for i := 0; i < b.N; i++ {
		test(8)
	}
}

// ~.14ms parallel
func BenchmarkPasswordVerificationRateParallelAES128(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			test(8)
		}
	})
}

func syscallPasswordCheck(file, password string) error {
	parg := fmt.Sprintf("-p%v", password)
	cmd := exec.Command("7z", "x", "-y", "-so", parg, file)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if strings.Contains(msg, "Wrong password") {
			return errors.New("Wrong Password")
		}
		return err
	}
	return nil
}

func BenchmarkSyscallPasswordCheck(b *testing.B) {
	for i := 0; i < b.N; i++ {
		syscallPasswordCheck("hello-aes.zip", "foobar")
	}
}

func TestMemoryPasswordVerification(t *testing.T) {
	guesses := make(chan string)
	go func() {
		for _, guess := range []string{"foobar", "batman", "aabaob", "golang"} {
			guesses <- guess
		}
	}()
	p := guessPassword("hello-aes.zip", guesses, nil)
	if p != "golang" {
		t.Errorf("expected password golang but got %v", p)
	}
}

func BenchmarkMemoryPasswordCheck(b *testing.B) {
	guesses := make(chan string)
	go func() {
		guesses <- "foobar"
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			guesses <- "foobar"
		}
		close(guesses)
	}()
	guessPassword("hello-aes.zip", guesses, nil)
}

func TestAlreadyCompleted(t *testing.T) {
	if v := alreadyCompleted([]charset{Lower}, []byte("abcdefg")); !v {
		t.Errorf("expected complete")
	}
	if v := alreadyCompleted([]charset{Lower}, []byte("A")); v {
		t.Errorf("expected incomplete")
	}
	if v := alreadyCompleted([]charset{Lower, flattenCharsets(Lower, Numbers)}, []byte("a1")); !v {
		t.Errorf("expected complete")
	}
	if v := alreadyCompleted([]charset{Lower, flattenCharsets(Lower, Numbers)}, []byte("a!")); v {
		t.Errorf("expected incomplete")
	}
}

func BenchmarkCharsetContainsCheck(b *testing.B) {
	charsets := []charset{
		Lower,
		flattenCharsets(Lower, Numbers),
		flattenCharsets(Lower, Upper, Numbers),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		alreadyCompleted(charsets, []byte("abc!6_"))
	}
}

// func BenchmarkGeneratePermutationsMemory(b *testing.B) {
// 	guesses := make(chan string)
// 	b.ReportAllocs()
// 	generatePermutations([]byte("aaljwi"), guesses)
// }
