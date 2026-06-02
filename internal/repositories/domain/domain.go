package domain

type PersonalData struct {
	UserID       string
	DOB          string
	ConsentGiven bool
}

type AstroProfile struct {
	ID           string
	UserID       string
	ProfileHash  string
	DOB          string
	ConsentGiven bool
}
