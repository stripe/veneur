package testhelpers

import (
	"math/rand"
	"time"
)

const (
	// DTK Terraform Test Account
	TestAccountID = 2520528
)

var letters = []rune("abcdefghijklmnopqrstuvwxyz")

// RandSeq is used to get a string made up of n random lowercase letters.
func RandSeq(n int) string {
	rand.Seed(time.Now().UnixNano())
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
