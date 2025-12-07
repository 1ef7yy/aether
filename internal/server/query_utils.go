package server

// extractQuery extracts the query string from message data
func extractQuery(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	queryEnd := len(data)
	for i, b := range data {
		if b == 0 {
			queryEnd = i
			break
		}
	}
	return string(data[:queryEnd])
}

// isBeginQuery checks if the query starts a transaction
func isBeginQuery(query string) bool {
	if len(query) < 5 {
		return false
	}
	q := query[:min(5, len(query))]
	return q == "BEGIN" || q == "begin" || q == "Start" || q == "start"
}

// isCommitQuery checks if the query commits a transaction
func isCommitQuery(query string) bool {
	if len(query) < 6 {
		return false
	}
	q := query[:min(6, len(query))]
	return q == "COMMIT" || q == "commit"
}

// isRollbackQuery checks if the query rolls back a transaction
func isRollbackQuery(query string) bool {
	if len(query) < 8 {
		return false
	}
	q := query[:min(8, len(query))]
	return q == "ROLLBACK" || q == "rollback"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
