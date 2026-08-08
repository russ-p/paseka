package energy

import "fmt"

// ValidateAddAmount checks CLI/API honey injection amounts.
func ValidateAddAmount(amount int) error {
	if amount <= 0 {
		return fmt.Errorf("energy amount must be positive")
	}
	return nil
}
