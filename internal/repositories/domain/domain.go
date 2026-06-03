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
	ProfileData  ProfileData
}

type ProfileData struct {
	SunSing   string `json:"sun_sign"`
	VenusSing string `json:"venus_sing"`
}
