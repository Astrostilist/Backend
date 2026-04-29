package alisa

// AstroProfile contains the user's basic astrological data,
// which is inserted into the prompt template.
type AstroProfile struct {
	UserID     string `json:"user_id,omitempty"`
	BirthDate  string `json:"birth_date,omitempty"`
	BirthTime  string `json:"birth_time,omitempty"`
	BirthPlace string `json:"birth_place,omitempty"`
}

func (p AstroProfile) IsZero() bool {
	return p.UserID == "" &&
		p.BirthDate == "" &&
		p.BirthTime == "" &&
		p.BirthPlace == ""
}
