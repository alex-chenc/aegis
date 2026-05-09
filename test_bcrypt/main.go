package main

import (
	"fmt"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	hash := "$2a$10$1fCzSdLlUke9KWPXgN/KXOEDBYSr6FEZwUJZ/m0hmDgtgAT4AkNyi"
	
	passwords := []string{"Admin@123", "admin@123", "Admin123", "admin123", "Admin@1234", "Aegis@123"}
	
	for _, pw := range passwords {
		err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw))
		fmt.Printf("%-15s match: %v\n", pw, err == nil)
	}
	
	// Generate a new hash for Admin@123 and verify it
	newHash, _ := bcrypt.GenerateFromPassword([]byte("Admin@123"), 10)
	fmt.Printf("\nNew hash for Admin@123: %s\n", string(newHash))
	err := bcrypt.CompareHashAndPassword(newHash, []byte("Admin@123"))
	fmt.Printf("Self-verify: %v\n", err == nil)
}
