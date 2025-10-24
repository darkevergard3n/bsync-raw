package utils

import (
	"crypto/rand"
	"math/big"
)

const (
	uppercaseLetters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	lowercaseLetters = "abcdefghijklmnopqrstuvwxyz"
	digits           = "0123456789"
	specialChars     = "!@#$%^&*()-_=+[]{}|;:,.<>?"
)

// GenerateRandomPassword generates a secure random password with the specified length.
// The password will contain at least one uppercase letter, one digit, and one special character.
// Default length is 8 characters if length < 8.
func GenerateRandomPassword(length int) (string, error) {
	if length < 8 {
		length = 8
	}

	// Ensure at least one character from each required category
	password := make([]byte, length)

	// Add at least one uppercase letter
	idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(uppercaseLetters))))
	if err != nil {
		return "", err
	}
	password[0] = uppercaseLetters[idx.Int64()]

	// Add at least one digit
	idx, err = rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
	if err != nil {
		return "", err
	}
	password[1] = digits[idx.Int64()]

	// Add at least one special character
	idx, err = rand.Int(rand.Reader, big.NewInt(int64(len(specialChars))))
	if err != nil {
		return "", err
	}
	password[2] = specialChars[idx.Int64()]

	// Fill the rest with random characters from all categories
	allChars := uppercaseLetters + lowercaseLetters + digits + specialChars
	for i := 3; i < length; i++ {
		idx, err = rand.Int(rand.Reader, big.NewInt(int64(len(allChars))))
		if err != nil {
			return "", err
		}
		password[i] = allChars[idx.Int64()]
	}

	// Shuffle the password to randomize the positions
	for i := length - 1; i > 0; i-- {
		j, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return "", err
		}
		password[i], password[j.Int64()] = password[j.Int64()], password[i]
	}

	return string(password), nil
}
