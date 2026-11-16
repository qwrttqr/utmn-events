package domain

// User represents a registered user independently of Telegram and database APIs.
type User struct {
	ID         int64
	TelegramID int64
	Username   string
	FirstName  string
	LastName   string
	Surname    string
	Role       string
}
