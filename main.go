package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	mapset "github.com/deckarep/golang-set"
)

var (
	Lower    = charRange('a', 'z')
	Upper    = charRange('A', 'Z')
	Numbers  = charRange('0', '9')
	Symbols  = []byte{'.', '/', '-', '_', '!', '?', '@', '#', '$', '%', '^', '&', '*', '+', '='}
	AllASCII = charRange(rune(32), rune(126))

	Combinations = [][][]byte{
		{Lower},
		{Lower, Numbers},
		{Lower, Upper, Numbers},
		{Lower, Upper, Numbers, Symbols},
		{AllASCII},
	}
	Sizes = []int{6, 1, 2, 3, 4, 5, 7, 8}

	MaxWorkers = 500
)

func charRange(start, end rune) []byte {
	bytes := make([]byte, int(end-start)+1)
	for i := 0; i <= int(end-start); i++ {
		bytes[i] = byte(int(start) + i)
	}
	return bytes
}

var ErrWrongPassword = errors.New("Wrong Password")
var ErrTooManyOpenFiles = errors.New("Too Many Open Files")

// guessPassword tries to unzip the file with the given password. Will return nil
// if the password was successful, WrongPasswordErr if there was a wrong password,
// and pass along any other errors
func guessPassword(file, password string) error {
	parg := fmt.Sprintf("-p%v", password)
	cmd := exec.Command("7z", "x", "-y", "-so", parg, file)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if strings.Contains(msg, "Wrong password") {
			return ErrWrongPassword
		} else if strings.Contains(msg, "too many open files") {
			return ErrTooManyOpenFiles
		}
		log.Printf("Error with command `7z x -y -so -p%v %v`: %v", password, file, err)
		return err
	}
	return nil
}

func charsets() [][]byte {
	charsets := make([][]byte, 0)
	for _, combination := range Combinations {
		charset := make([]byte, 0)
		for _, c := range combination {
			charset = append(charset, c...)
		}
		charsets = append(charsets, charset)
	}
	return charsets
}

func main() {
	file := os.Args[1]
	if file == "" {
		log.Fatalf("USAGE: pwd [path-to-zip-file]")
	}

	guessed := mapset.NewSet()
	// running these things in parallel can quickly eat up all the file handles for the process
	// so we limit the number of parallel goroutines
	wg := sync.WaitGroup{}

	for _, size := range Sizes {
		for _, charset := range charsets() {
			guesses := make(chan string)
			log.Printf("Trying %d character set of length %d", len(charset), size)
			go eachPermutation(charset, size, guesses)
			for i := 0; i < MaxWorkers; i++ {
				wg.Add(1)
				go func(file string) {
					defer wg.Done()
					for guess := range guesses {
					Retry:
						if guessed.Contains(guess) {
							continue
						}
						err := guessPassword(file, guess)
						if err == nil {
							log.Printf("Got 'em! The password is: %v", guess)
							os.Exit(0)
							return
						}
						if errors.Is(err, ErrWrongPassword) {
							// expected
							continue
						}
						if errors.Is(err, ErrTooManyOpenFiles) {
							log.Print("Too many open files, backing off...")
							time.Sleep(100)
							goto Retry
						}
						log.Panic(err)
					}
				}(file)
			}
			wg.Wait()
		}
	}
	log.Fatal(`No luck ¯\_(ツ)_/¯`)
}

type charsetIterator struct {
	charset []byte
	index   int
	next    *charsetIterator
}

func (c *charsetIterator) Next(out []byte) (done bool) {
	char := c.charset[c.index]
	out[0] = char
	if c.next == nil {
		c.index++
	} else {
		nextDone := c.next.Next(out[1:])
		if nextDone {
			c.index++
		}
	}
	if c.index == len(c.charset) {
		c.index = 0
		return true
	}
	return false
}

func eachPermutation(charset []byte, size int, guesses chan<- string) {
	iterators := make([]charsetIterator, size)
	for i := 0; i < size-1; i++ {
		iterators[i] = charsetIterator{charset: charset, next: &iterators[i+1]}
	}
	iterators[size-1] = charsetIterator{charset: charset}
	done := false
	guess := make([]byte, size)
	for i := 0; !done; i++ {
		done = iterators[0].Next(guess)
		s := string(guess)
		guesses <- s
		if i%1000000 == 0 {
			log.Printf("guess %010dM: %v", i/1000000, s)
		}
	}
	close(guesses)
}
