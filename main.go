package main

import (
	"bytes"
	"errors"
	"io/ioutil"
	"log"
	"os"
	"sync"

	zipw "github.com/alexmullins/zip"
)

type charset []byte

var (
	Lower    = charRange('a', 'z')
	Upper    = charRange('A', 'Z')
	Numbers  = charRange('0', '9')
	Symbols  = charset{'.', '/', '-', '_', '!', '?', '@', '#', '$', '%', '^', '&', '*', '+', '='}
	AllASCII = charRange(rune(32), rune(126))

	Combinations = []charset{
		Lower,
		Upper,
		flattenCharsets(Upper, Numbers),
		flattenCharsets(Lower, Numbers),
		flattenCharsets(Lower, Numbers, Symbols),
		flattenCharsets(Lower, Upper, Numbers, Symbols),
		AllASCII,
	}
	Sizes = []int{6, 1, 2, 3, 4, 5, 7, 8}
)

const (
	MaxWorkers = 50
	LogEvery   = 100000
)

func flattenCharsets(sets ...charset) charset {
	cs := make(charset, 0)
	for _, s := range sets {
		for _, c := range s {
			if !bytes.ContainsRune(cs, rune(c)) {
				cs = append(cs, c)
			}
		}
	}
	return cs
}

func charRange(start, end rune) charset {
	bytes := make(charset, int(end-start)+1)
	for i := 0; i <= int(end-start); i++ {
		bytes[i] = byte(int(start) + i)
	}
	return bytes
}

// guessPassword tries to unzip the file with the given password. Will return nil
// if the password was successful, WrongPasswordErr if there was a wrong password,
// and pass along any other errors
func guessPassword(file string, guesses <-chan string, match chan<- string) string {
	f, err := os.Open(file)
	if err != nil {
		log.Panicf("Couldn't open file: %v", err)
	}
	defer f.Close()
	data, err := ioutil.ReadAll(f)
	if err != nil {
		log.Panicf("Couldn't read file: %v", err)
	}
	buf := bytes.NewReader(data)
	r, err := zipw.NewReader(buf, int64(len(data)))
	if err != nil {
		log.Panicf("Couldn't create zip reader: %v", err)
	}
	var zf *zipw.File
	for i := 0; i < len(r.File); i++ {
		zf = r.File[i]
		if zf.IsEncrypted() && !zf.FileInfo().IsDir() {
			break
		}
	}
	for guess := range guesses {
		zf.SetPassword(guess)
		_, err := zf.Open()
		if errors.Is(err, zipw.ErrPassword) {
			continue
		}
		if err != nil {
			log.Panicf("problem guessing password: %v", err)
		}
		// there's a 1 in 2^16 chance that the password verification
		// byte matches for any password. we need to actually
		// read the value
		rc, err := zf.Open()
		if err != nil {
			log.Panicf("open err: %v", err)
		}
		b := make([]byte, 1)
		_, err = rc.Read(b)
		if errors.Is(err, zipw.ErrAuthentication) {
			continue
		}
		if match != nil {
			match <- guess
		}
		return guess
	}
	return ""
}

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("USAGE: pwd [path-to-zip-file]")
	}
	file := os.Args[1]
	var skipahead string
	if len(os.Args) > 2 {
		skipahead = os.Args[2]
	}

	match := make(chan string)
	go func() {
		password := <-match
		log.Printf("Got 'em! The password is: %v", password)
		os.Exit(0)
	}()

	wg := sync.WaitGroup{}

	guesses := make(chan string)
	go generatePermutations([]byte(skipahead), guesses)
	for i := 0; i < MaxWorkers; i++ {
		wg.Add(1)
		go guessPassword(file, guesses, match)
	}
	wg.Wait()
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

func (cs charset) Contains(b byte) bool {
	for _, c := range cs {
		if c == b {
			return true
		}
	}
	return false
}

func (cs charset) ContainsAll(b []byte) bool {
	for _, c := range b {
		if !cs.Contains(c) {
			return false
		}
	}
	return true
}

func alreadyCompleted(completedCharsets []charset, guess []byte) bool {
	for _, charset := range completedCharsets {
		if charset.ContainsAll(guess) {
			return true
		}
	}
	return false
}

func generatePermutations(skipahead []byte, guesses chan<- string) {
	skipping := len(skipahead) > 0
	index := uint64(0)
	for _, size := range Sizes {
		completedCharsets := make([]charset, 0, len(Combinations))
		for _, charset := range Combinations {
			log.Printf("Trying %d character set of length %d", len(charset), size)
			iterators := make([]charsetIterator, size)
			for i := 0; i < size-1; i++ {
				iterators[i] = charsetIterator{charset: charset, next: &iterators[i+1]}
			}
			iterators[size-1] = charsetIterator{charset: charset}
			done := false
			guess := make([]byte, size)
			for !done {
				done = iterators[0].Next(guess)
				index++
				if alreadyCompleted(completedCharsets, guess) {
					continue
				}
				if skipping {
					if bytes.Equal(skipahead, guess) {
						log.Printf("skipped ahead to %v", string(skipahead))
						skipping = false
					}
					continue
				}
				s := string(guess)
				if index%LogEvery == 0 {
					log.Printf("guess %015d: %v", index, s)
				}
				guesses <- s
			}
			completedCharsets = append(completedCharsets, charset)
		}
	}
	close(guesses)
}
