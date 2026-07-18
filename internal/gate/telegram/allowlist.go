package telegram

// Allowed reports whether the Telegram user and chat may interact with the gate.
// Both allow_from and chat_ids must match; everyone else is silently ignored.
func Allowed(cfg Config, userID, chatID int64) bool {
	if userID == 0 || chatID == 0 {
		return false
	}
	if !containsInt64(cfg.AllowFrom, userID) {
		return false
	}
	return containsInt64(cfg.ChatIDs, chatID)
}

func containsInt64(list []int64, v int64) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
