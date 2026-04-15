package domain

type User struct {
	UserID       string
	DateOfBirth  string
	ConsentGiven bool
}

// PersonalData – это персональные данные, подлежащие защите
type PersonalData struct {
	UserID string
	DOB    string
}
