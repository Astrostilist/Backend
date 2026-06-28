package domain

import "errors"

type PersonalData struct {
	UserID       string
	DOB          string
	DOBTime      string
	ConsentGiven bool
}

type AstroProfile struct {
	ID           string
	UserID       string
	ProfileHash  string
	DOB          string
	DOBTime      string
	ConsentGiven bool
	ProfileData  ProfileData
}

type ProfileData struct {
	Sun     string `json:"sun"`
	Moon    string `json:"moon"`
	Venus   string `json:"venus"`
	Mars    string `json:"mars"`
	Jupiter string `json:"jupiter"`
	Saturn  string `json:"saturn"`
	Rahu    string `json:"rahu"`
	Ketu    string `json:"ketu"`
	Mercury string `json:"mercury"`
	Neptune string `json:"neptune"`
	Uranus  string `json:"uranus"`
	Pluto   string `json:"pluto"`
}

const (
	Sun     = "sun"
	Moon    = "moon"
	Venus   = "venus"
	Mars    = "mars"
	Jupiter = "jupiter"
	Saturn  = "saturn"
	Rahu    = "rahu"
	Ketu    = "ketu"
	Mercury = "mercury"
	Neptune = "neptune"
	Uranus  = "uranus"
	Pluto   = "pluto"
)

// Zodiac Sign IDs for the natal chart
const (
	AriesID       = "aries"
	TaurusID      = "taurus"
	GeminiID      = "gemini"
	CancerID      = "cancer"
	LeoID         = "leo"
	VirgoID       = "virgo"
	LibraID       = "libra"
	ScorpioID     = "scorpio"
	SagittariusID = "sagittarius"
	CapricornID   = "capricorn"
	AquariusID    = "aquarius"
	PiscesID      = "pisces"
)

// ErrNotFound возвращается, когда данные не найдены.
var ErrNotFound = errors.New("user not found")
