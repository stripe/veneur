package testhelpers

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"
)

const (
	// DTK Terraform Test Account
	TestAccountID = 2520528
)

var (
	letters    = []rune("abcdefghijklmnopqrstuvwxyz")
	TestUserID = os.Getenv("NEW_RELIC_TEST_USER_ID")
)

// RandSeq is used to get a string made up of n random lowercase letters.
func RandSeq(n int) string {
	rand.Seed(time.Now().UnixNano())
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func GetTestUserID() (int, error) {
	userID := os.Getenv("NEW_RELIC_TEST_USER_ID")

	if userID == "" {
		return 0, fmt.Errorf("failed to get test user ID due to undefined environment variable %s", "NEW_RELIC_TEST_USER_ID")
	}

	n, err := strconv.Atoi(userID)
	if err != nil {
		return 0, err
	}

	return n, nil
}
