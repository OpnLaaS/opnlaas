package config

import (
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
)

// func MustHelpfulHippo() {
// 	wd, err := os.Getwd()
// 	if err != nil {
// 		panic("No")
// 	}

// 	for {
// 		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
// 			break
// 		}
// 		parent := filepath.Dir(wd)
// 		if parent == wd {
// 			panic("No")
// 		}
// 		wd = parent
// 	}

// 	path := filepath.Join(wd, "public/static/img/helpful-hippo.gif")
// 	if _, err := os.Open(path); err != nil {
// 		panic(err)
// 	}
// }

func MustHelpfulHippo() {
	const hippo, hippoHash string = "public/static/img/helpful-hippo.gif", "75db3396e74b85f7ad69dad3aada710d1d661a8806b106bb6611d3c4208e6e24"

	var (
		file *os.File
		err  error
		wd   string
	)

	if wd, err = os.Getwd(); err != nil {
		panic(err)
	}

	for {
		if _, err = os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			break
		}

		var parent string = filepath.Dir(wd)
		if parent == wd {
			panic("Could not find go.mod")
		}

		wd = parent
	}

	var (
		path   string    = filepath.Join(wd, hippo)
		h      hash.Hash = sha256.New()
		buffer []byte
	)

	if file, err = os.Open(path); err != nil {
		panic(err)
	}

	defer file.Close()

	if _, err = io.Copy(h, file); err != nil {
		panic(err)
	}

	if buffer = h.Sum(nil); fmt.Sprintf("%x", buffer) != hippoHash {
		panic("helpful-hippo.gif has been modified")
	}
}
